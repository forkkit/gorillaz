package stream

import (
	"context"
	"time"
)

type Event struct {
	Ctx        context.Context
	Key, Value []byte
	AckFunc    func() error
}

func (e *Event) Ack() error {
	if e.AckFunc == nil {
		return nil
	}
	return e.AckFunc()
}

// The key type is unexported to prevent collisions with context keys defined in
// other packages.
type key string

const eventTypeKey = key("event_type")
const eventTypeVersionKey = key("event_type_version")
const eventTimeNs = key("event_time_ns")
const streamTimestampNs = key("stream_timestamp_ns")
const originStreamTimestampNs = key("origin_stream_timestamp_ns")
const deadlineKey = key("deadline")
const pendingKey = key("pending")
const subjectKey = key("subject")
const streamKey = key("stream")
const consumerSeqKey = key("consumerSeq")
const streamSeqKey = key("streamSeq")

// StreamTimestamp returns the time when the event was sent from the producer in Epoch in nanoseconds
func StreamTimestamp(e *Event) int64 {
	if e.Ctx == nil {
		return 0
	}
	ts := e.Ctx.Value(streamTimestampNs)
	if ts != nil {
		if res, ok := ts.(int64); ok {
			return res
		}
	}
	return 0
}

// OriginStreamTimestamp returns the time when the event was sent from the first producer in Epoch in nanoseconds
func OriginStreamTimestamp(e *Event) int64 {
	if e.Ctx == nil {
		return 0
	}
	ts := e.Ctx.Value(originStreamTimestampNs)
	if ts != nil {
		if res, ok := ts.(int64); ok {
			return res
		}
	}
	return 0
}

// CtxWithDeadline returns the event context with deadline applied
func (evt *Event) CtxWithDeadline() (context.Context, context.CancelFunc) {
	v := evt.Ctx.Value(deadlineKey)
	if v == nil {
		return evt.Ctx, func() {}
	}
	deadline, ok := v.(int64)

	// if the deadline is set to 0, consider it as no deadline
	if !ok || deadline == 0 {
		return evt.Ctx, func() {}
	}
	sec := deadline / 1000000000
	ns := deadline - (sec * 1000000000)

	t := time.Unix(sec, ns)
	return context.WithDeadline(evt.Ctx, t)
}

// Deadline returns the event deadline as a unix timestamp in ns if available
func (evt *Event) Deadline() (int64, bool) {
	v := evt.Ctx.Value(deadlineKey)
	if v == nil {
		return 0, false
	}
	ts := v.(int64)
	if ts == 0 {
		return 0, false
	}
	return ts, true
}

// SetEventTimestamp stores in the event the timestamp of when the event happened (as opposite as when it was streamed).
// use this function to store values such as when an observation was recorded
func SetEventTimestamp(evt *Event, t time.Time) {
	if evt.Ctx == nil {
		evt.Ctx = context.Background()
	}
	evt.Ctx = context.WithValue(evt.Ctx, eventTimeNs, t.UnixNano())
}

// SetEventTime stores in the event the timestamp of when the event happened (as opposite as when it was streamed).
// use this function to store values such as when an observation was recorded
func (evt *Event) SetEventTime(t time.Time) {
	if evt.Ctx == nil {
		evt.Ctx = context.Background()
	}
	evt.Ctx = context.WithValue(evt.Ctx, eventTimeNs, t.UnixNano())
}

// SetOriginStreamTime sets the time when the event was sent from the first producer
func (evt *Event) SetOriginStreamTime(t time.Time) {
	if evt.Ctx == nil {
		evt.Ctx = context.Background()
	}
	evt.Ctx = context.WithValue(evt.Ctx, originStreamTimestampNs, t.UnixNano())
}

// SetStreamTime sets the time when the event was sent from the producer
func (evt *Event) SetStreamTime(t time.Time) {
	if evt.Ctx == nil {
		evt.Ctx = context.Background()
	}
	evt.Ctx = context.WithValue(evt.Ctx, streamTimestampNs, t.UnixNano())
}

// EventTimestamp returns the event creation time Epoch in nanoseconds
func EventTimestamp(e *Event) int64 {
	if e.Ctx == nil {
		return 0
	}
	ts := e.Ctx.Value(eventTimeNs)
	if ts != nil {
		if res, ok := ts.(int64); ok {
			return res
		}
	}
	return 0
}

func (evt *Event) SetEventTypeStr(eventType string) {
	if evt.Ctx == nil {
		evt.Ctx = context.Background()
	}
	evt.Ctx = context.WithValue(evt.Ctx, eventTypeKey, eventType)
}

func (evt *Event) EventTypeStr() string {
	if evt.Ctx == nil {
		return ""
	}
	v := evt.Ctx.Value(eventTypeKey)
	if v == nil {
		return ""
	}
	if eType, ok := v.(string); ok {
		return eType
	}
	return ""
}

func (evt *Event) SetEventTypeVersionStr(version string) {
	if evt.Ctx == nil {
		evt.Ctx = context.Background()
	}
	evt.Ctx = context.WithValue(evt.Ctx, eventTypeVersionKey, version)
}

func (evt *Event) EventTypeVersionStr() string {
	if evt.Ctx == nil {
		return ""
	}
	v := evt.Ctx.Value(eventTypeVersionKey)
	if v == nil {
		return ""
	}
	if eType, ok := v.(string); ok {
		return eType
	}
	return ""
}

func (evt *Event) SetPending(pending int) {
	if evt.Ctx == nil {
		evt.Ctx = context.Background()
	}
	evt.Ctx = context.WithValue(evt.Ctx, pendingKey, pending)
}

func (evt *Event) Pending() int {
	if evt.Ctx == nil {
		return 0
	}
	v := evt.Ctx.Value(pendingKey)
	if v == nil {
		return 0
	}
	if resultType, ok := v.(int); ok {
		return resultType
	}
	return 0
}

func (evt *Event) SetConsumerSeq(consumerSeq int) {
	if evt.Ctx == nil {
		evt.Ctx = context.Background()
	}
	evt.Ctx = context.WithValue(evt.Ctx, consumerSeqKey, consumerSeq)
}

func (evt *Event) ConsumerSeq() int {
	if evt.Ctx == nil {
		return 0
	}
	v := evt.Ctx.Value(consumerSeqKey)
	if v == nil {
		return 0
	}
	if resultType, ok := v.(int); ok {
		return resultType
	}
	return 0
}

func (evt *Event) SetStreamSeq(consumerSeq int) {
	if evt.Ctx == nil {
		evt.Ctx = context.Background()
	}
	evt.Ctx = context.WithValue(evt.Ctx, streamSeqKey, consumerSeq)
}

func (evt *Event) StreamSeq() int {
	if evt.Ctx == nil {
		return 0
	}
	v := evt.Ctx.Value(streamSeqKey)
	if v == nil {
		return 0
	}
	if resultType, ok := v.(int); ok {
		return resultType
	}
	return 0
}

func (evt *Event) SetSubject(version string) {
	if evt.Ctx == nil {
		evt.Ctx = context.Background()
	}
	evt.Ctx = context.WithValue(evt.Ctx, subjectKey, version)
}

func (evt *Event) Subject() string {
	if evt.Ctx == nil {
		return ""
	}
	v := evt.Ctx.Value(subjectKey)
	if v == nil {
		return ""
	}
	if resultType, ok := v.(string); ok {
		return resultType
	}
	return ""
}

func (evt *Event) SetStream(version string) {
	if evt.Ctx == nil {
		evt.Ctx = context.Background()
	}
	evt.Ctx = context.WithValue(evt.Ctx, streamKey, version)
}

func (evt *Event) Stream() string {
	if evt.Ctx == nil {
		return ""
	}
	v := evt.Ctx.Value(streamKey)
	if v == nil {
		return ""
	}
	if resultType, ok := v.(string); ok {
		return resultType
	}
	return ""
}
