// Package events lets "Then" steps wait on pipeline events instead of sleeping.
//
// A Waiter attaches ordered (ephemeral, unacked) JetStream consumers filtered on one
// video_id to VIDEO_EVENTS and VIDEO_DLQ and appends every message to a Log. Steps then
// block in Log.WaitFor until a predicate matches or the context deadline passes. Ordered
// consumers start at DeliverAll, so events published before the waiter was attached
// are seen as well.
package events

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/framepulse/framepulse/tests/acceptance/internal/contracts"
)

// Event is one stored JetStream message as observed by the test.
type Event struct {
	Stream    string
	Subject   string
	Header    nats.Header
	Data      []byte
	StreamSeq uint64
	Timestamp time.Time // stream timestamp (publish time as stored by the server)
	Received  time.Time // when the test observed it
}

func (e Event) MsgID() string  { return e.Header.Get(contracts.HeaderMsgID) }
func (e Event) Schema() string { return e.Header.Get(contracts.HeaderSchema) }

func (e Event) String() string {
	return fmt.Sprintf("%s#%d %s msg-id=%q schema=%q", e.Stream, e.StreamSeq, e.Subject, e.MsgID(), e.Schema())
}

// Predicate selects events.
type Predicate func(Event) bool

func OnSubject(s string) Predicate     { return func(e Event) bool { return e.Subject == s } }
func WithMsgID(id string) Predicate    { return func(e Event) bool { return e.MsgID() == id } }
func WithSchema(name string) Predicate { return func(e Event) bool { return e.Schema() == name } }
func InStream(s string) Predicate      { return func(e Event) bool { return e.Stream == s } }

func All(ps ...Predicate) Predicate {
	return func(e Event) bool {
		for _, p := range ps {
			if !p(e) {
				return false
			}
		}
		return true
	}
}

// Log is an append-only, concurrency-safe event list with blocking waits.
type Log struct {
	mu      sync.Mutex
	events  []Event
	changed chan struct{} // closed and replaced on every append
	err     error         // terminal consumer error, if any
}

func NewLog() *Log { return &Log{changed: make(chan struct{})} }

func (l *Log) Append(e Event) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if e.Received.IsZero() {
		e.Received = time.Now()
	}
	l.events = append(l.events, e)
	close(l.changed)
	l.changed = make(chan struct{})
}

// Fail records a terminal consumer error; waits return it instead of timing out.
func (l *Log) Fail(err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.err == nil {
		l.err = err
	}
	close(l.changed)
	l.changed = make(chan struct{})
}

// Snapshot returns a copy of all events observed so far.
func (l *Log) Snapshot() []Event {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]Event(nil), l.events...)
}

// Count returns how many observed events match p.
func (l *Log) Count(p Predicate) int {
	n := 0
	for _, e := range l.Snapshot() {
		if p(e) {
			n++
		}
	}
	return n
}

// WaitFor blocks until an event matching p has been observed (including events observed
// before the call) and returns the first such event.
func (l *Log) WaitFor(ctx context.Context, p Predicate) (Event, error) {
	evs, err := l.WaitForN(ctx, 1, p)
	if err != nil {
		return Event{}, err
	}
	return evs[0], nil
}

// WaitForN blocks until at least n matching events have been observed.
func (l *Log) WaitForN(ctx context.Context, n int, p Predicate) ([]Event, error) {
	if n <= 0 {
		return nil, fmt.Errorf("WaitForN: n must be positive, got %d", n)
	}
	for {
		l.mu.Lock()
		var matched []Event
		for _, e := range l.events {
			if p(e) {
				matched = append(matched, e)
			}
		}
		changed, termErr, total := l.changed, l.err, len(l.events)
		l.mu.Unlock()

		if len(matched) >= n {
			return matched, nil
		}
		if termErr != nil {
			return matched, fmt.Errorf("event consumer failed after %d events (%d/%d matched): %w", total, len(matched), n, termErr)
		}
		select {
		case <-ctx.Done():
			return matched, fmt.Errorf("timed out waiting for %d matching events (%d matched, %d observed): %w",
				n, len(matched), total, ctx.Err())
		case <-changed:
		}
	}
}

// WaitForWithin is WaitFor bounded by timeout.
func (l *Log) WaitForWithin(ctx context.Context, timeout time.Duration, p Predicate) (Event, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return l.WaitFor(ctx, p)
}
