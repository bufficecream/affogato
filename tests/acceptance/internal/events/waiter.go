package events

import (
	"context"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/framepulse/framepulse/tests/acceptance/internal/contracts"
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
	msg := &nats.Msg{Subject: subject, Data: data, Header: nats.Header{}}
	msg.Header.Set(contracts.HeaderContentType, contracts.ContentTypeProto)
	msg.Header.Set(contracts.HeaderSchema, schema)
	msg.Header.Set(contracts.HeaderProducer, "acceptance-tests/dev")
	return js.PublishMsg(ctx, msg, jetstream.WithMsgID(msgID))
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
