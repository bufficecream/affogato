package events

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/framepulse/framepulse/tests/acceptance/internal/contracts"
)

// startJetStream runs an in-process nats-server with JetStream and creates the
// VIDEO_EVENTS / VIDEO_DLQ streams and the HLS_INDEX bucket with the contract's
// subjects and duplicate windows (B1). No external infrastructure is used.
func startJetStream(t *testing.T) jetstream.JetStream {
	t.Helper()
	srv, err := server.NewServer(&server.Options{Host: "127.0.0.1", Port: -1, JetStream: true, StoreDir: t.TempDir(), NoLog: true, NoSigs: true})
	if err != nil {
		t.Fatal(err)
	}
	go srv.Start()
	if !srv.ReadyForConnections(10 * time.Second) {
		t.Fatal("nats-server not ready")
	}
	t.Cleanup(srv.Shutdown)
	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(nc.Close)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name: contracts.StreamEvents, Subjects: []string{"video.chunks.>", "video.lifecycle.>"},
		Duplicates: 10 * time.Minute,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name: contracts.StreamDLQ, Subjects: []string{"dlq.>"}, Duplicates: 10 * time.Minute,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{Bucket: contracts.KVHlsIndex, History: 1}); err != nil {
		t.Fatal(err)
	}
	return js
}

func mustPublish(t *testing.T, js jetstream.JetStream, subject, msgID, schema string) *jetstream.PubAck {
	t.Helper()
	ack, err := Publish(context.Background(), js, subject, msgID, schema, []byte("payload"))
	if err != nil {
		t.Fatalf("publish %s: %v", subject, err)
	}
	return ack
}

func TestWaiterSeesAllSubjectsOfOneVideoOnly(t *testing.T) {
	js := startJetStream(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	v, _ := contracts.NewVideoID()
	other, _ := contracts.NewVideoID()

	// Published before the waiter is attached: must still be observed (DeliverAll).
	mustPublish(t, js, contracts.SubjectChunkUploaded(v), contracts.MsgIDUploaded(v, 0), contracts.SchemaVideoChunkUploaded)
	mustPublish(t, js, contracts.SubjectChunkUploaded(other), contracts.MsgIDUploaded(other, 0), contracts.SchemaVideoChunkUploaded)

	w, err := Watch(ctx, js, v)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	mustPublish(t, js, contracts.SubjectHlsReady(other), contracts.MsgIDHlsReady(other, "src", 0), contracts.SchemaChunkHlsReady)
	mustPublish(t, js, contracts.SubjectHlsReady(v), contracts.MsgIDHlsReady(v, "src", 0), contracts.SchemaChunkHlsReady)
	mustPublish(t, js, contracts.SubjectAIIndexed(v), contracts.MsgIDAIIndexed(v, 0), contracts.SchemaChunkTranscriptIndexed)
	mustPublish(t, js, contracts.SubjectIngestCompleted(v), contracts.MsgIDCompleted(v), contracts.SchemaVideoIngestCompleted)
	mustPublish(t, js, contracts.SubjectDLQ(contracts.ConsumerHls, v), contracts.MsgIDDLQ(contracts.ConsumerHls, 3), contracts.SchemaDeadLetter)
	mustPublish(t, js, contracts.SubjectDLQ(contracts.ConsumerHls, other), contracts.MsgIDDLQ(contracts.ConsumerHls, 4), contracts.SchemaDeadLetter)

	want := []string{
		contracts.MsgIDUploaded(v, 0),
		contracts.MsgIDHlsReady(v, "src", 0),
		contracts.MsgIDAIIndexed(v, 0),
		contracts.MsgIDCompleted(v),
		contracts.MsgIDDLQ(contracts.ConsumerHls, 3),
	}
	for _, id := range want {
		e, err := w.WaitFor(ctx, WithMsgID(id))
		if err != nil {
			t.Fatalf("waiting for %s: %v (seen %v)", id, err, w.Snapshot())
		}
		if e.StreamSeq == 0 || e.Timestamp.IsZero() {
			t.Errorf("%s: missing stream metadata: %+v", id, e)
		}
	}
	// The DLQ message is in the second stream; let it settle, then check nothing foreign leaked.
	if _, err := w.WaitFor(ctx, InStream(contracts.StreamDLQ)); err != nil {
		t.Fatal(err)
	}
	if n := len(w.Snapshot()); n != len(want) {
		t.Fatalf("observed %d events, want %d: %v", n, len(want), w.Snapshot())
	}
}

func TestDuplicateMsgIDStoredOnce(t *testing.T) {
	js := startJetStream(t)
	ctx := context.Background()
	v, _ := contracts.NewVideoID()
	subj, id := contracts.SubjectChunkUploaded(v), contracts.MsgIDUploaded(v, 5)
	first := mustPublish(t, js, subj, id, contracts.SchemaVideoChunkUploaded)
	second := mustPublish(t, js, subj, id, contracts.SchemaVideoChunkUploaded)
	if first.Duplicate || !second.Duplicate {
		t.Fatalf("dedup flags: first=%v second=%v", first.Duplicate, second.Duplicate)
	}
	n, err := StoredCount(ctx, js, contracts.StreamEvents, subj)
	if err != nil || n != 1 {
		t.Fatalf("StoredCount = %d, %v", n, err)
	}
}

func TestWatchKey(t *testing.T) {
	js := startJetStream(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	kv, err := js.KeyValue(ctx, contracts.KVHlsIndex)
	if err != nil {
		t.Fatal(err)
	}
	v, _ := contracts.NewVideoID()
	key := contracts.HlsIndexKey(v, "src")
	if _, err := kv.Put(ctx, key, []byte("processing")); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		_, _ = kv.Put(context.Background(), key, []byte("finalized"))
	}()
	val, err := WatchKey(ctx, kv, key, func(b []byte) (bool, error) { return string(b) == "finalized", nil })
	if err != nil || string(val) != "finalized" {
		t.Fatalf("WatchKey = %q, %v", val, err)
	}

	short, cancel2 := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel2()
	if _, err := WatchKey(short, kv, key, func([]byte) (bool, error) { return false, nil }); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want timeout, got %v", err)
	}
}

func TestWatchRejectsInvalidVideoID(t *testing.T) {
	js := startJetStream(t)
	if _, err := Watch(context.Background(), js, "V.>"); err == nil {
		t.Fatal("wildcard-bearing video_id accepted as filter")
	}
}
