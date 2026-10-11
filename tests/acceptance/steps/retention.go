package steps

import (
	"context"
	"fmt"

	"github.com/affogato/affogato/tests/acceptance/internal/contracts"
)

const fRet = "F7 retention"

// Feature 7: Retention configuration.
func registerRetention(r Registrar) {
	r.Step(`^the operator inspects the deployed NATS and MinIO configuration$`,
		func(ctx context.Context) error {
			// TODO: js.Stream(name).Info for each stream; s3.ExpirationDays per bucket; store in w.Values.
			return pending(fRet, "read stream configs and bucket lifecycles")
		})
	r.Step(`^streams ((?:"[^"]+"(?:, | and )?)+) each have max_age (\d+) days$`,
		func(ctx context.Context, list string, days int) error {
			names := quotedList(list)
			for _, n := range names {
				switch n {
				case contracts.StreamEvents, contracts.StreamDLQ, contracts.StreamAdvisories:
				default:
					return fmt.Errorf("unknown stream %q", n)
				}
			}
			// TODO: Config.MaxAge == days*24h for each.
			return pending(fRet, "streams %v max_age == %d d", names, days)
		})
	r.Step(`^bucket "([^"]+)" expires objects after (\d+) days$`,
		func(ctx context.Context, bucket string, days int) error {
			// TODO: s3.ExpirationDays(bucket) == days.
			return pending(fRet, "bucket %s expiry == %d d", bucket, days)
		})
	r.Step(`^bucket "([^"]+)" has no expiration rule$`,
		func(ctx context.Context, bucket string) error {
			// TODO: s3.ExpirationDays(bucket) == 0 (abort-multipart rule allowed).
			return pending(fRet, "bucket %s has no expiry", bucket)
		})
	r.Step(`^the "([^"]+)" expiry is longer than the "([^"]+)" max_age$`,
		func(ctx context.Context, bucket, stream string) error {
			// TODO: days(bucket)*24h > stream max_age.
			return pending(fRet, "expiry(%s) > max_age(%s)", bucket, stream)
		})
}
