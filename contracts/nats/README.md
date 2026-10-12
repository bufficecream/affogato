# NATS JetStream configuration

Source of truth for the streams, consumers and KV bucket in
[`docs/architecture-review.md`](../../docs/architecture-review.md) B1. The Helm chart (H1)
renders these files into NACK resources unchanged; services only check that the objects exist.
`contracts/check-config.sh` applies them to a real `nats-server` and compares the read-back.

| Directory | Shape | Apply by hand |
|---|---|---|
| `streams/<NAME>.json` | server `StreamConfig` (`$JS.API.STREAM.CREATE`) | `nats stream add --config streams/VIDEO_EVENTS.json` |
| `consumers/<durable>.json` | server `ConsumerConfig` | `nats consumer add VIDEO_EVENTS --config consumers/ai-indexer.json` |
| `kv/<BUCKET>.json` | `KeyValueConfig` (nats.go `jetstream`) | `nats kv add HLS_INDEX --history 1 --max-value-size 1048576 --storage file --replicas 3` (the CLI has no `--config` for KV) |

Rules for these files:
- **Durations are integer nanoseconds** (server JSON), e.g. `max_age` 7 d = `604800000000000`.
- **Replicas are the production value (3).** A single-node server rejects them
  (`replicas > 1 not supported in non-clustered mode`, 10074); `values-kind.yaml` and
  `check-config.sh` override to 1. Consumers leave `num_replicas` unset (0 = inherit the stream).
- **A consumer's stream** is `metadata."affogato.stream"`; `ConsumerConfig` has no stream field.
  The server keeps the key, so `nats consumer info` shows it.
- **Omit a field to get the server default.** The server drops `"compression": "none"` on
  read-back, so uncompressed streams leave `compression` out.
- File name = `name` / `durable_name` / `bucket`.

## Mapping to NACK CRDs (`jetstream.nats.io/v1beta2`)

NACK uses camelCase and Go duration strings. Fields that differ beyond camelCase:

| Server JSON (these files) | NACK CRD | Note |
|---|---|---|
| `num_replicas` (stream) | `replicas` | |
| `max_age`, `duplicate_window`, `ack_wait` (ns integer) | `maxAge`, `duplicateWindow`, `ackWait` (string, e.g. `168h`, `10m`, `15s`) | convert ns → duration string |
| `ttl` (KV, ns) | `ttl` (string) | not used here |
| `allow_rollup_hdrs` | `allowRollup` | not used here |
| `storage` default `file` | `storage` default **`memory`** | the chart must always render `storage` |
| `durable_name` | `durableName`, plus `streamName` (from `metadata."affogato.stream"`) | |
| `compression` (KV `bool`) | `compression` (bool) | |

Other used fields map by camelCase only: `subjects`, `retention`, `maxBytes`, `maxMsgSize`,
`discard`, `denyDelete`, `denyPurge`, `allowDirect`, `compression`, `description`,
`filterSubjects`, `deliverPolicy`, `ackPolicy`, `maxDeliver`, `maxAckPending`, `replayPolicy`,
`metadata`, `bucket`, `history`, `maxValueSize`.
