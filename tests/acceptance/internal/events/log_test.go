package events

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

func ev(subject, msgID string) Event {
	h := nats.Header{}
	h.Set("Nats-Msg-Id", msgID)
	return Event{Subject: subject, Header: h}
}

func TestWaitForSeesPastEvents(t *testing.T) {
	l := NewLog()
	l.Append(ev("a", "1"))
	got, err := l.WaitForWithin(context.Background(), time.Second, WithMsgID("1"))
	if err != nil || got.Subject != "a" {
		t.Fatalf("WaitFor = %v, %v", got, err)
	}
}

func TestWaitForWakesOnAppend(t *testing.T) {
	l := NewLog()
	go func() {
		time.Sleep(20 * time.Millisecond)
		l.Append(ev("x", "other"))
		l.Append(ev("a", "want"))
	}()
	got, err := l.WaitForWithin(context.Background(), 2*time.Second, WithMsgID("want"))
	if err != nil || got.Subject != "a" {
		t.Fatalf("WaitFor = %v, %v", got, err)
	}
}

func TestWaitForTimesOut(t *testing.T) {
	l := NewLog()
	l.Append(ev("a", "1"))
	start := time.Now()
	_, err := l.WaitForWithin(context.Background(), 50*time.Millisecond, WithMsgID("never"))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want deadline error, got %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("timeout not honoured")
	}
}

func TestWaitForNAndCount(t *testing.T) {
	l := NewLog()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); l.Append(ev("s", "dup")) }()
	}
	evs, err := l.WaitForN(context.Background(), 50, All(OnSubject("s"), WithMsgID("dup")))
	if err != nil || len(evs) != 50 {
		t.Fatalf("WaitForN = %d, %v", len(evs), err)
	}
	wg.Wait()
	if n := l.Count(OnSubject("s")); n != 50 {
		t.Fatalf("Count = %d", n)
	}
	if _, err := l.WaitForN(context.Background(), 0, OnSubject("s")); err == nil {
		t.Fatal("n=0 accepted")
	}
}

func TestFailUnblocksWaiters(t *testing.T) {
	l := NewLog()
	boom := errors.New("consumer deleted")
	go func() { time.Sleep(20 * time.Millisecond); l.Fail(boom) }()
	_, err := l.WaitForWithin(context.Background(), 5*time.Second, WithMsgID("x"))
	if !errors.Is(err, boom) {
		t.Fatalf("want consumer error, got %v", err)
	}
}

func TestSnapshotIsACopy(t *testing.T) {
	l := NewLog()
	l.Append(ev("a", "1"))
	s := l.Snapshot()
	s[0].Subject = "mutated"
	if l.Snapshot()[0].Subject != "a" {
		t.Fatal("Snapshot exposes internal slice")
	}
}
