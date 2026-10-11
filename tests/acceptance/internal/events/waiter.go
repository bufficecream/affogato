package events

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/affogato/affogato/tests/acceptance/internal/contracts"
)

// Waiter feeds one video's VIDEO_EVENTS and VIDEO_DLQ messages into a Log.
type Waiter struct {
	*Log
	VideoID  string
	consumes []jetstream.ConsumeContext
}

// Watch attaches ordered consumers for videoID. Call Stop when the scenario ends.
func Watch(ctx context.Context, js jetstream.JetStream, videoID string) (*Waiter, error) {
	if err := contracts.ValidateVideoID(videoID); err != nil {
		return nil, err
	}
	w := &Waiter{Log: NewLog(), VideoID: videoID}
	targets := []struct {
		stream  string
		filters []string
	}{
		{contracts.StreamEvents, contracts.EventFilterSubjects(videoID)},
		{contracts.StreamDLQ, contracts.DLQFilterSubjects(videoID)},
	}
	for _, t := range targets {
		if err := w.attach(ctx, js, t.stream, t.filters); err != nil {
			w.Stop()
			return nil, err
		}
	}
	return w, nil
}

func (w *Waiter) attach(ctx context.Context, js jetstream.JetStream, stream string, filters []string) error {
	cons, err := js.OrderedConsumer(ctx, stream, jetstream.OrderedConsumerConfig{
		FilterSubjects: filters,
		DeliverPolicy:  jetstream.DeliverAllPolicy,
	})
	if err != nil {
		return fmt.Errorf("ordered consumer on %s %v: %w", stream, filters, err)
	}
	cc, err := cons.Consume(func(m jetstream.Msg) {
		ev := Event{Stream: stream, Subject: m.Subject(), Header: cloneHeader(m.Headers()), Data: append([]byte(nil), m.Data()...)}
		if md, err := m.Metadata(); err == nil {
			ev.StreamSeq = md.Sequence.Stream
			ev.Timestamp = md.Timestamp
		}
		w.Append(ev)
	}, jetstream.ConsumeErrHandler(func(_ jetstream.ConsumeContext, err error) {
		// Ordered consumers recreate themselves on most errors; only record terminal ones.
		if errors.Is(err, jetstream.ErrConsumerDeleted) || errors.Is(err, nats.ErrConnectionClosed) {
			w.Fail(fmt.Errorf("%s consumer: %w", stream, err))
		}
	}))
	if err != nil {
		return fmt.Errorf("consume %s: %w", stream, err)
	}
	w.consumes = append(w.consumes, cc)
	return nil
}

// Stop detaches all consumers. Safe to call more than once.
func (w *Waiter) Stop() {
	for _, cc := range w.consumes {
		cc.Stop()
	}
	w.consumes = nil
}

func cloneHeader(h nats.Header) nats.Header {
	out := nats.Header{}
	for k, v := range h {
		out[k] = append([]string(nil), v...)
	}
	return out
}

// Publish sends a test event with the contract headers and waits for the PubAck.
func Publish(ctx context.Context, js jetstream.JetStream, subject, msgID, schema string, data []byte) (*jetstream.PubAck, error) {
	return PublishWithHeaders(ctx, js, subject, msgID, schema, data, nil)
}

// PublishWithFault publishes like Publish and adds one Af-Test-Fault header (spec 2.5),
// e.g. contracts.TestFaultValue(contracts.FaultExitOnReceive, contracts.ConsumerAI, 0).
// The value is validated with the same grammar the workers apply.
func PublishWithFault(ctx context.Context, js jetstream.JetStream, subject, msgID, schema string, data []byte, faultValue string) (*jetstream.PubAck, error) {
	if _, err := contracts.ParseTestFaultValue(faultValue); err != nil {
		return nil, err
	}
	return PublishWithHeaders(ctx, js, subject, msgID, schema, data, nats.Header{contracts.HeaderTestFault: {faultValue}})
}

// contractHeaders are set by PublishWithHeaders itself and cannot be overridden by extra.
var contractHeaders = map[string]bool{
	contracts.HeaderMsgID: true, contracts.HeaderContentType: true,
	contracts.HeaderSchema: true, contracts.HeaderProducer: true,
}

// PublishWithHeaders publishes with the contract headers plus extra headers (for example
// Af-Test-Fault). Extra headers may not replace contract headers.
func PublishWithHeaders(ctx context.Context, js jetstream.JetStream, subject, msgID, schema string, data []byte, extra nats.Header) (*jetstream.PubAck, error) {
	msg := &nats.Msg{Subject: subject, Data: data, Header: nats.Header{}}
	for k, vs := range extra {
		if contractHeaders[http.CanonicalHeaderKey(k)] || contractHeaders[k] {
			return nil, fmt.Errorf("extra header %q would override a contract header", k)
		}
		for _, v := range vs {
			msg.Header.Add(k, v)
		}
	}
	msg.Header.Set(contracts.HeaderContentType, contracts.ContentTypeProto)
	msg.Header.Set(contracts.HeaderSchema, schema)
	msg.Header.Set(contracts.HeaderProducer, "acceptance-tests/dev")
	return js.PublishMsg(ctx, msg, jetstream.WithMsgID(msgID))
}

// ConsumerState is the part of a durable consumer's info the redelivery and DLQ steps use.
type ConsumerState struct {
	AckFloorStream  uint64 // every stream seq <= this is acked or terminated
	DeliveredStream uint64 // highest stream seq delivered so far
	DeliveredTotal  uint64 // deliveries including redeliveries (consumer sequence)
	NumAckPending   int
	NumRedelivered  int
	NumPending      uint64
}

// ConsumerSnapshot reads a durable consumer's state (e.g. "ai-indexer" on VIDEO_EVENTS).
func ConsumerSnapshot(ctx context.Context, js jetstream.JetStream, stream, durable string) (ConsumerState, error) {
	c, err := js.Consumer(ctx, stream, durable)
	if err != nil {
		return ConsumerState{}, fmt.Errorf("consumer %s/%s: %w", stream, durable, err)
	}
	info, err := c.Info(ctx)
	if err != nil {
		return ConsumerState{}, fmt.Errorf("consumer %s/%s info: %w", stream, durable, err)
	}
	return ConsumerState{
		AckFloorStream:  info.AckFloor.Stream,
		DeliveredStream: info.Delivered.Stream,
		DeliveredTotal:  info.Delivered.Consumer,
		NumAckPending:   info.NumAckPending,
		NumRedelivered:  info.NumRedelivered,
		NumPending:      info.NumPending,
	}, nil
}

// WaitAckFloor blocks until the durable consumer's ack floor reaches streamSeq, i.e. the
// message was finally acked or terminated by the worker (Feature 5 redelivery, Feature 4
// Term). Consumer info has no watch API, so this polls every poll interval (not a fixed
// sleep: it returns as soon as the condition holds, bounded by ctx).
func WaitAckFloor(ctx context.Context, js jetstream.JetStream, stream, durable string, streamSeq uint64, poll time.Duration) (ConsumerState, error) {
	if poll <= 0 {
		poll = 250 * time.Millisecond
	}
	t := time.NewTicker(poll)
	defer t.Stop()
	for {
		st, err := ConsumerSnapshot(ctx, js, stream, durable)
		if err == nil && st.AckFloorStream >= streamSeq {
			return st, nil
		}
		select {
		case <-ctx.Done():
			if err != nil {
				return st, fmt.Errorf("ack floor of %s/%s never reached %d: %w (last error: %v)", stream, durable, streamSeq, ctx.Err(), err)
			}
			return st, fmt.Errorf("ack floor of %s/%s is %d, never reached %d: %w", stream, durable, st.AckFloorStream, streamSeq, ctx.Err())
		case <-t.C:
		}
	}
}

// StoredCount returns how many messages the stream currently stores on exactly subject.
func StoredCount(ctx context.Context, js jetstream.JetStream, stream, subject string) (uint64, error) {
	s, err := js.Stream(ctx, stream)
	if err != nil {
		return 0, err
	}
	info, err := s.Info(ctx, jetstream.WithSubjectFilter(subject))
	if err != nil {
		return 0, err
	}
	return info.State.Subjects[subject], nil
}

// WatchKey blocks until the KV entry for key satisfies pred, using a KV watch (itself an
// ordered consumer) rather than polling. The current value, if any, is checked first.
// pred receives nil for a deleted/purged key.
func WatchKey(ctx context.Context, kv jetstream.KeyValue, key string, pred func(value []byte) (bool, error)) ([]byte, error) {
	watcher, err := kv.Watch(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("watch %s/%s: %w", kv.Bucket(), key, err)
	}
	defer func() { _ = watcher.Stop() }()
	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timed out waiting for %s/%s: %w", kv.Bucket(), key, ctx.Err())
		case entry, ok := <-watcher.Updates():
			if !ok {
				return nil, fmt.Errorf("watch %s/%s closed", kv.Bucket(), key)
			}
			if entry == nil { // end of initial values marker
				continue
			}
			var val []byte
			if entry.Operation() == jetstream.KeyValuePut {
				val = entry.Value()
			}
			done, err := pred(val)
			if err != nil {
				return val, err
			}
			if done {
				return val, nil
			}
		}
	}
}
