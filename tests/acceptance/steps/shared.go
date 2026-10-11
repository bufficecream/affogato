package steps

import (
	"context"
)

// Steps used by more than one feature.
func registerShared(r Registrar) {
	// Feature 1 "Out-of-order chunks…" and Feature 2 "Completion arrives before the last chunk…".
	r.Step(`^"VideoIngestCompleted" with chunk_count (\d+) is delivered$`,
		func(ctx context.Context, count int) error {
			if count <= 0 {
				return errInvalid("chunk_count must be positive")
			}
			// TODO: encode VideoIngestCompleted{video_id: V, chunk_count} with the Codec and
			// publish on video.lifecycle.completed.V with Msg-Id completed:V (events.Publish);
			// then wait until the event is in the waiter log (delivered = stored; delivery to
			// hls-stream-worker is observable only via HLS_INDEX expected_chunk_count).
			return pending("shared", "publish VideoIngestCompleted chunk_count=%d and wait for HLS_INDEX expected_chunk_count", count)
		})
}

type errInvalid string

func (e errInvalid) Error() string { return string(e) }
