package gorillaz

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/skysoft-atm/gorillaz/stream"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

// PullJetstream returns the next subject and event for the given stream and consumer
// If the event is processed successfully, it must be acknowledge to get a new message. If the message is not acknowledge, then PullJetstream will return the same event multiple times
// If no message is available after the ctx timeout, then an error nats.ErrTimeout is returned with an empty subject and an event nil
func (g *Gaz) PullJetstream(ctx context.Context, stream string, consumer string) (subject string, event *stream.Event, err error) {
	subj := "$JS.API.CONSUMER.MSG.NEXT." + g.AddStreamEnvIfMissing(stream) + "." + consumer
	msg, err := g.NatsConn.RequestWithContext(ctx, subj, nil)
	if err != nil {
		return "", nil, err
	}
	e := msgToEvent(msg)
	e.AckFunc = func() error {
		return msg.Respond(nil)
	}
	return msg.Subject, e, nil
}

type JSApiConsumerGetNextRequest struct {
	Expires time.Time `json:"expires,omitempty"`
	Batch   int       `json:"batch,omitempty"`
	NoWait  bool      `json:"no_wait,omitempty"`
}

type pullOptions struct {
	batchSize                 int
	closeOnEndOfStreamReached bool
	ackImmediately            bool
}

type PullOption func(*pullOptions)

func CloseOnEndOfStream(c bool) func(o *pullOptions) {
	return func(o *pullOptions) {
		o.closeOnEndOfStreamReached = c
	}
}

func AckImmediately(c bool) func(o *pullOptions) {
	return func(o *pullOptions) {
		o.ackImmediately = c
	}
}

func BatchSize(s int) func(o *pullOptions) {
	return func(o *pullOptions) {
		o.batchSize = s
	}
}

func (g *Gaz) AddStreamEnvIfMissing(streamName string) string {
	if !strings.HasPrefix(streamName, g.Env) {
		return g.Env + "-" + streamName
	}
	return streamName
}

func (g *Gaz) AddConsumerEnvIfMissing(consumerName string) string {
	if !strings.HasPrefix(consumerName, g.Env) {
		return g.Env + consumerName
	}
	return consumerName
}

// Pulls messages from a stream by batch, the batch size is configurable
// A consumer with the given name must exists before calling this method.
func (g *Gaz) PullJetstreamBatch(ctx context.Context, streamName string, consumer string, options ...PullOption) (<-chan *stream.Event, <-chan error) {
	o := pullOptions{
		batchSize:                 100,
		closeOnEndOfStreamReached: false,
		ackImmediately:            false,
	}
	for _, opt := range options {
		opt(&o)
	}
	eventChan := make(chan *stream.Event, o.batchSize)
	errChan := make(chan error, 1)

	subj := "$JS.API.CONSUMER.MSG.NEXT." + g.AddStreamEnvIfMissing(streamName) + "." + g.AddConsumerEnvIfMissing(consumer)

	req := JSApiConsumerGetNextRequest{
		Batch: o.batchSize,
	}
	jreq, err := json.Marshal(req)
	if err != nil {
		errChan <- err
		return eventChan, errChan
	}

	go func() {
		sub, err := g.NatsConn.SubscribeSync(nats.NewInbox())
		if err != nil {
			Log.Warn("subscribe failed", zap.Error(err))
			errChan <- err
			return
		}
		defer func() {
			err := sub.Unsubscribe()
			if err != nil {
				Log.Warn("Could not unsubscribe", zap.Error(err))
			}
			close(errChan)
		}()
		err = g.NatsConn.PublishMsg(&nats.Msg{Subject: subj, Reply: sub.Subject, Data: jreq})
		if err != nil {
			errChan <- err
			return
		}
		received := 0

		for {
			msg, err := sub.NextMsgWithContext(ctx)
			if err != nil {
				errChan <- err
				return
			}
			received++
			if len(msg.Data) == 0 && msg.Header != nil && msg.Header.Get("Status") != "" {
				errChan <- fmt.Errorf("status %s received from Jetstream", msg.Header.Get("Status"))
				_ = msg.Ack()
				return
			}
			event := msgToEvent(msg)

			if o.ackImmediately {
				err = msg.Ack()
				if err != nil {
					errChan <- fmt.Errorf("could not ack message: %w", err)
					return
				}
			} else {
				event.AckFunc = func() error {
					return msg.Ack()
				}
			}
			eventChan <- event

			if event.Pending() == 0 && o.closeOnEndOfStreamReached {
				close(eventChan)
				return
			}

			// for now, don't be too clever by pulling too much in advance
			if received == o.batchSize {
				err = g.NatsConn.PublishMsg(&nats.Msg{Subject: subj, Reply: sub.Subject, Data: jreq})
				if err != nil {
					errChan <- err
					return
				}
				received = 0
			}
		}
	}()
	return eventChan, errChan
}

// MsgHandler handles received events from Nats
// If NatsConsumerOpts.AutoAck is set, if MsgHandler returns no error, the message will be acknowledged. If an error is returned, the event won't be acknowledged.
type MsgHandler func(subject string, event *stream.Event) (reply *stream.Event, err error)

type NatsConsumerOpts struct {
	autoAck        bool
	tracingEnabled bool
	queue          string
}

type NatsConsumerOpt func(n *NatsConsumerOpts)

// WithAutoAck automatic acknowledge message received if MsgHandle returns no error
func WithAutoAck() NatsConsumerOpt {
	return func(o *NatsConsumerOpts) {
		o.autoAck = true
	}
}

func WithTracing() NatsPublishOpt {
	return func(o *NatsPublishOpts) {
		o.tracingEnabled = true
	}
}

// WithQueue configures Nats Queue consumer
func WithQueue(queue string) NatsConsumerOpt {
	return func(o *NatsConsumerOpts) {
		o.queue = queue
	}
}

// SubscribeNatsSubject subscribes to a Nats stream, and forward received messages to handler
// An error is returned if the subscription fails, but not when the connection with Nats is interrupted
func (g *Gaz) SubscribeNatsSubject(subject string, handler MsgHandler, opts ...NatsConsumerOpt) (*NatsSubscription, error) {
	if g.addEnvPrefixToNats {
		subject = g.Env + "." + subject
	}
	c := &NatsConsumerOpts{
		autoAck:        false,
		tracingEnabled: false,
	}
	for _, o := range opts {
		o(c)
	}
	if g.NatsConn == nil {
		return nil, fmt.Errorf("gorillaz nats connection is nil, cannot consume stream")
	}

	do := func(m *nats.Msg) {
		e := msgToEvent(m)

		// if there is no auto ack, then the user is responsible for calling event.Ack
		if !c.autoAck && m.Reply != "" {
			e.AckFunc = func() error {
				return m.Respond(nil)
			}
		}

		response, err := handler(m.Subject, e)

		if err == nil {
			if m.Reply != "" && c.autoAck {
				Log.Debug("ack", zap.String("subject", subject), zap.String("reply", m.Reply))
				if err := m.Respond(nil); err != nil {
					// TODO: not great for consumer, he may receive the same event multiple times and really be aware
					Log.Error("failed to ack event", zap.Error(err))
				}
				return
			}
		}

		if response != nil && m.Reply != "" {
			if response.Ctx == nil {
				response.Ctx = context.Background()
			}
			stream.FillTracingSpan(response, e)

			metadata, err := stream.EventMetadata(response)
			if err != nil {
				Log.Error("failed to create metadata from event", zap.Error(err))
			}

			r := &stream.StreamEvent{Metadata: metadata, Key: response.Key, Value: response.Value}
			b, err := proto.Marshal(r)
			if err != nil {
				Log.Error("failed to marshal response", zap.Error(err))
				return
			}

			Log.Debug("reply", zap.String("subject", subject), zap.String("reply", m.Reply))
			if err := m.Respond(b); err != nil {
				Log.Error("failed to ack event", zap.Error(err))
			}
		}
	}

	var err error
	var sub *nats.Subscription

	if c.queue == "" {
		sub, err = g.NatsConn.Subscribe(subject, func(m *nats.Msg) {
			do(m)
		})
	} else {
		sub, err = g.NatsConn.QueueSubscribe(subject, c.queue, func(m *nats.Msg) {
			do(m)
		})
	}

	if err == nil {
		return &NatsSubscription{n: sub}, nil
	}
	return nil, err
}

type NatsPublishOpts struct {
	tracingEnabled bool
}

type NatsPublishOpt func(opts *NatsPublishOpts)

func WithNatsTracingEnabled() NatsPublishOpt {
	return func(o *NatsPublishOpts) {
		o.tracingEnabled = true
	}
}

func (g *Gaz) NatsPublish(subject string, e *stream.Event, opts ...NatsPublishOpt) error {
	if g.addEnvPrefixToNats {
		subject = g.Env + "." + subject
	}
	conf := &NatsPublishOpts{}

	for _, opt := range opts {
		opt(conf)
	}
	metadata, err := stream.EventMetadata(e)
	if err != nil {
		return err
	}
	evt := stream.StreamEvent{Key: e.Key, Value: e.Value, Metadata: metadata}
	b, err := proto.Marshal(&evt)
	if err != nil {
		return err
	}
	return g.NatsConn.Publish(subject, b)
}

func (g *Gaz) NatsRequest(ctx context.Context, subject string, e *stream.Event, opts ...NatsPublishOpt) (*stream.Event, error) {
	if g.addEnvPrefixToNats {
		subject = g.Env + "." + subject
	}
	conf := &NatsPublishOpts{}

	for _, opt := range opts {
		opt(conf)
	}
	metadata, err := stream.EventMetadata(e)
	if err != nil {
		return nil, err
	}
	evt := stream.StreamEvent{Key: e.Key, Value: e.Value, Metadata: metadata}
	b, err := proto.Marshal(&evt)
	if err != nil {
		return nil, err
	}
	msg, err := g.NatsConn.RequestWithContext(ctx, subject, b)
	if err != nil {
		return nil, err
	}
	return msgToEvent(msg), nil
}

func msgToEvent(msg *nats.Msg) *stream.Event {
	var evt stream.StreamEvent
	value := msg.Data
	var key []byte
	ctx := context.Background()

	// try to deserialize object
	err := proto.Unmarshal(msg.Data, &evt)
	if err == nil {
		key = evt.Key
		value = evt.Value
		ctx = stream.Ctx(evt.Metadata)
	}
	e := &stream.Event{Ctx: ctx, Key: key, Value: value, AckFunc: func() error { return nil }}
	meta, err := msg.JetStreamMetaData()
	if err == nil && meta != nil {
		e.SetPending(meta.Pending)
		e.SetConsumerSeq(meta.ConsumerSeq)
		e.SetStreamSeq(meta.StreamSeq)
		e.SetSubject(msg.Subject)
		e.SetStream(meta.Stream)
	}
	return e
}

type NatsSubscription struct {
	n *nats.Subscription
}

func (n *NatsSubscription) Unsubscribe() error {
	return n.n.Unsubscribe()
}

func (n *NatsSubscription) Subject() string {
	return n.n.Subject
}

func (n *NatsSubscription) Queue() string {
	return n.n.Queue
}

// mustInitNats connects to nats broker with address addr, or panic
// if successful, g.NatsConn is set
func (g *Gaz) mustInitNats(addr string) {
	timeout := time.Duration(g.Viper.GetUint64("nats.connect_timeout_ms")) * time.Millisecond
	var err error
	g.NatsConn, err = nats.Connect(addr, nats.Timeout(timeout))
	if err != nil {
		Log.Panic("failed to initialize nats connection", zap.Error(err))
	}
}
