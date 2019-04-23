package gorillaz

import (
	"context"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/skysoft-atm/gorillaz/stream"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/balancer/roundrobin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/resolver/manual"
	"google.golang.org/grpc/status"
	"math"
	"strings"
	"sync"
	"time"
)

var mu sync.RWMutex
var authority string

type ConsumerConfig struct {
	BufferLen      int // BufferLen is the size of the channel of the consumer
	onConnected    func(streamName string)
	onDisconnected func(streamName string)
	UseGzip        bool
}

type StreamEndpointConfig struct {
	backoffMaxDelay time.Duration
}

type Consumer struct {
	StreamName string
	EvtChan    chan *stream.Event
	config     *ConsumerConfig
}

type StreamEndpoint struct {
	target    string
	endpoints []string
	config    *StreamEndpointConfig
	conn      *grpc.ClientConn
}

func defaultConsumerConfig() *ConsumerConfig {
	return &ConsumerConfig{
		BufferLen: 256,
	}
}

func defaultStreamEndpointConfig() *StreamEndpointConfig {
	return &StreamEndpointConfig{
		backoffMaxDelay: 5 * time.Second,
	}
}

func BackoffMaxDelay(duration time.Duration) StreamEndpointConfigOpt {
	return func(config *StreamEndpointConfig) {
		config.backoffMaxDelay = duration
	}

}

type ConsumerConfigOpt func(*ConsumerConfig)

type StreamEndpointConfigOpt func(config *StreamEndpointConfig)

type EndpointType uint8

const (
	DNSEndpoint = EndpointType(iota)
	IPEndpoint
)

func NewStreamEndpoint(endpointType EndpointType, endpoints []string, opts ...StreamEndpointConfigOpt) (*StreamEndpoint, error) {
	// TODO: hacky hack to create a resolver to use with round robin
	mu.Lock()
	r, _ := manual.GenerateAndRegisterManualResolver()
	mu.Unlock()

	addresses := make([]resolver.Address, len(endpoints))
	for i := 0; i < len(endpoints); i++ {
		addresses[i] = resolver.Address{Addr: endpoints[i]}
	}
	r.InitialAddrs(addresses)
	target := r.Scheme() + ":///stream"

	config := defaultStreamEndpointConfig()
	for _, opt := range opts {
		opt(config)
	}
	conn, err := grpc.Dial(target, grpc.WithInsecure(), grpc.WithBalancerName(roundrobin.Name), grpc.WithInsecure(),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(&gogoCodec{})),
		grpc.WithBackoffMaxDelay(config.backoffMaxDelay))
	if err != nil {
		return nil, err
	}
	endpoint := &StreamEndpoint{
		config:    config,
		endpoints: endpoints,
		target:    target,
		conn:      conn,
	}

	return endpoint, nil
}

func (se *StreamEndpoint) Close() error {
	return se.conn.Close()
}

func (se *StreamEndpoint) ConsumeStream(streamName string, opts ...ConsumerConfigOpt) *Consumer {

	config := defaultConsumerConfig()
	for _, opt := range opts {
		opt(config)
	}

	ch := make(chan *stream.Event, config.BufferLen)
	c := &Consumer{
		StreamName: streamName,
		EvtChan:    ch,
		config:     config,
	}

	var monitoringHolder = consumerMonitoring(streamName, se.endpoints)

	go func() {
		for se.conn.GetState() != connectivity.Shutdown {
			waitTillReady(se)

			client := stream.NewStreamClient(se.conn)
			req := &stream.StreamRequest{Name: streamName}

			var callOpts []grpc.CallOption
			if config.UseGzip {
				callOpts = append(callOpts, grpc.UseCompressor(gzip.Name))
			}
			callOpts = append(callOpts)
			st, err := client.Stream(context.Background(), req, callOpts...)
			if err != nil {
				Log.Warn("Error while creating stream", zap.String("stream", streamName), zap.Error(err))
				if se.conn.GetState() == connectivity.Ready {
					//weird, let's wait before recreating the stream
					time.Sleep(5 * time.Second)
				}
				continue
			}
			if config.onConnected != nil {
				config.onConnected(streamName)
			}

			// at this point, the GRPC connection is established with the server
			firstEvent := true
			for {
				streamEvt, err := st.Recv()

				if err != nil {
					Log.Warn("received error on stream", zap.String("stream", c.StreamName), zap.Error(err))
					if e, ok := status.FromError(err); ok {
						switch e.Code() {
						case codes.PermissionDenied:
						case codes.ResourceExhausted:
						case codes.Unavailable:
						case codes.Unimplemented:
						case codes.NotFound:
						case codes.Unauthenticated:
						case codes.Unknown: // stream name probably does not exists
							time.Sleep(5 * time.Second)
						}
					}
					break
				}

				// if first event received successfully, set the status to connected.
				// we need to do it here because setting up a GRPC connection is not enough, the server can still return us an error
				if firstEvent {
					firstEvent = false
					monitoringHolder.conGauge.Set(1)
				}
				Log.Debug("event received", zap.String("stream", streamName))
				monitorDelays(monitoringHolder, streamEvt)

				evt := &stream.Event{
					Key:   streamEvt.Key,
					Value: streamEvt.Value,
					Ctx:   stream.MetadataToContext(*streamEvt.Metadata),
				}
				c.EvtChan <- evt
			}
			monitoringHolder.conGauge.Set(0)
			if config.onDisconnected != nil {
				config.onDisconnected(streamName)
			}

		}
		Log.Info("Stream closed", zap.String("stream", c.StreamName))
		close(c.EvtChan)

	}()
	return c
}

func monitorDelays(monitoringHolder consumerMonitoringHolder, streamEvt *stream.StreamEvent) {
	monitoringHolder.receivedCounter.Inc()
	nowMs := float64(time.Now().UnixNano()) / 1000000.0
	streamTimestamp := streamEvt.Metadata.StreamTimestamp
	if streamTimestamp > 0 {
		// convert from ns to ms
		monitoringHolder.delaySummary.Observe(math.Max(0, nowMs-float64(streamTimestamp)/1000000.0))
	}
	eventTimestamp := streamEvt.Metadata.EventTimestamp
	if eventTimestamp > 0 {
		monitoringHolder.eventDelaySummary.Observe(math.Max(0, nowMs-float64(eventTimestamp)/1000000.0))
	}
	originTimestamp := streamEvt.Metadata.OriginStreamTimestamp
	if originTimestamp > 0 {
		monitoringHolder.originDelaySummary.Observe(math.Max(0, nowMs-float64(originTimestamp)/1000000.0))
	}
}

func waitTillReady(se *StreamEndpoint) {
	for state := se.conn.GetState(); state != connectivity.Ready; state = se.conn.GetState() {
		Log.Debug("Waiting for stream endpoint connection to be ready", zap.Strings("endpoint", se.endpoints))
		se.conn.WaitForStateChange(context.Background(), state)
	}
}

//// SetDNSAddr be used to define the DNS server to use for DNS endpoint type, in format "IP:PORT"
//func SetDNSAddr(addr string) {
//	mu.Lock()
//	defer mu.Unlock()
//	authority = addr
//}
//
//func grpcTarget(endpointType EndpointType, endpoints []string) string {
//	switch endpointType {
//	case IPEndpoint:
//		// TODO: hacky hack to create a resolver for list of IP addresses
//		mu.Lock()
//		r, _ := manual.GenerateAndRegisterManualResolver()
//		mu.Unlock()
//
//		addresses := make([]resolver.Address, len(endpoints))
//		for i := 0; i < len(endpoints); i++ {
//			addresses[i] = resolver.Address{Addr: endpoints[i]}
//		}
//		r.InitialAddrs(addresses)
//		return r.Scheme() + ":///stream"
//	case DNSEndpoint:
//		if len(endpoints) != 1 {
//			panic("DNS Grpc endpointType expect only 1 endpoint address, but got " + strconv.Itoa(len(endpoints)))
//		}
//		return "dns://" + authority + "/" + endpoints[0]
//	default:
//		panic("unknown Grpc EndpointType " + strconv.Itoa(int(endpointType)))
//	}
//	return ""
//}

type consumerMonitoringHolder struct {
	receivedCounter    prometheus.Counter
	conCounter         prometheus.Counter
	conGauge           prometheus.Gauge
	delaySummary       prometheus.Summary
	originDelaySummary prometheus.Summary
	eventDelaySummary  prometheus.Summary
}

// map of metrics registered to Prometheus
// it's here because we cannot register twice to Prometheus the metrics with the same label
// if we register several consumers on the same stream, we must be sure we don't register the metrics twice
var consMonitoringMu sync.Mutex
var consumerMonitorings = make(map[string]consumerMonitoringHolder)

func consumerMonitoring(streamName string, endpoints []string) consumerMonitoringHolder {
	consMonitoringMu.Lock()
	defer consMonitoringMu.Unlock()

	if m, ok := consumerMonitorings[streamName]; ok {
		return m
	}
	m := consumerMonitoringHolder{
		receivedCounter: promauto.NewCounter(prometheus.CounterOpts{
			Name: "stream_consumer_received_events",
			Help: "The total number of events received",
			ConstLabels: prometheus.Labels{
				"stream":    streamName,
				"endpoints": strings.Join(endpoints, ","),
			},
		}),

		conCounter: promauto.NewCounter(prometheus.CounterOpts{
			Name: "stream_consumer_connection_attempts",
			Help: "The total number of connections to the stream",
			ConstLabels: prometheus.Labels{
				"stream":    streamName,
				"endpoints": strings.Join(endpoints, ","),
			},
		}),

		conGauge: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "stream_consumer_connected",
			Help: "1 if connected, otherwise 0",
			ConstLabels: prometheus.Labels{
				"stream":    streamName,
				"endpoints": strings.Join(endpoints, ","),
			},
		}),

		delaySummary: promauto.NewSummary(prometheus.SummaryOpts{
			Name:       "stream_consumer_delay_ms",
			Help:       "distribution of delay between when messages are sent to from the consumer and when they are received, in milliseconds",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
			ConstLabels: prometheus.Labels{
				"stream":    streamName,
				"endpoints": strings.Join(endpoints, ","),
			},
		}),

		originDelaySummary: promauto.NewSummary(prometheus.SummaryOpts{
			Name:       "stream_consumer_origin_delay_ms",
			Help:       "distribution of delay between when messages were created by the first producer in the chain of streams, and when they are received, in milliseconds",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
			ConstLabels: prometheus.Labels{
				"stream":    streamName,
				"endpoints": strings.Join(endpoints, ","),
			},
		}),
		eventDelaySummary: promauto.NewSummary(prometheus.SummaryOpts{
			Name:       "stream_consumer_event_delay_ms",
			Help:       "distribution of delay between when messages were created and when they are received, in milliseconds",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
			ConstLabels: prometheus.Labels{
				"stream":    streamName,
				"endpoints": strings.Join(endpoints, ","),
			},
		}),
	}
	consumerMonitorings[streamName] = m
	return m
}

type gogoCodec struct{}

// Marshal returns the wire format of v.
func (c *gogoCodec) Marshal(v interface{}) ([]byte, error) {
	var req = v.(*stream.StreamRequest)
	return req.Marshal()
}

// Unmarshal parses the wire format into v.
func (c *gogoCodec) Unmarshal(data []byte, v interface{}) error {
	evt := v.(*stream.StreamEvent)
	return evt.Unmarshal(data)
}

func (c *gogoCodec) Name() string {
	return "gogoCodec"
}
