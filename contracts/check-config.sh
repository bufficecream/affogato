#!/usr/bin/env bash
# Validate the NATS and Qdrant contract files against real servers (milestone C3).
#
# Starts a throwaway nats-server (JetStream), a Qdrant and a nats-box toolbox container on a
# private Docker network (no host ports are published, so parallel runs never collide),
# applies every file under contracts/nats and contracts/qdrant twice (the second apply proves
# the apply is idempotent), reads each object back and compares every field the file sets.
# It also checks the retention rules of Feature 7 (tests/acceptance/features/07_retention.feature)
# and that AF_ADVISORIES really captures a MAX_DELIVERIES advisory. Everything it starts is
# removed on exit.
#
# Usage:  contracts/check-config.sh          (needs bash and docker; jq/curl/nats run in nats-box)
# Output: one PASS/FAIL line per check and a SUMMARY line. Exit 0 only if every check passed.
# Env:    NATS_IMAGE, NATS_BOX_IMAGE, QDRANT_IMAGE override the refs from tests/kind/images.txt.
#
# SC2016: single-quoted '$...' here is always a jq variable or the literal NATS subject prefix
# "$JS.", never a shell expansion.
# shellcheck disable=SC2016
set -euo pipefail

CONTRACTS_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd "$CONTRACTS_DIR/.." && pwd)
IMAGES_FILE="$REPO_ROOT/tests/kind/images.txt"

# --- Constants -----------------------------------------------------------------------------

# Production replica count written in the files; a single-node server rejects it
# (error 10074), so it is overridden to 1 here exactly like values-kind.yaml does.
FILE_REPLICAS=3
APPLY_REPLICAS=1

SEVEN_DAYS_NS=604800000000000
# Planned af-raw lifecycle expiry. It is configured by the H1 init Job
# (deploy/helm/affogato/MILESTONES.md, H1), not by these files; keep the two equal. Feature 7
# requires it to be longer than the VIDEO_DLQ max_age so every dead letter can be redriven.
AF_RAW_EXPIRY_DAYS=10

REQUIRED_STREAMS="VIDEO_EVENTS VIDEO_DLQ AF_ADVISORIES"
REQUIRED_CONSUMERS="hls-stream-worker ai-indexer"
REQUIRED_KV="HLS_INDEX"
REQUIRED_COLLECTIONS="transcripts_me5s384_v1"
REQUIRED_ALIASES="transcripts"

# Recursive "every field in the file equals the server value" check. Objects are compared
# key by key (extra server fields are ignored); scalars and arrays must be equal. Prints one
# line per mismatch, nothing when equal.
DIFF_JQ='
def diff($want; $got; $path):
  if ($want | type) == "object" then
    $want | to_entries[] | .key as $k
    | diff(.value; (if ($got | type) == "object" then $got[$k] else null end); $path + [$k])
  elif $want == $got then empty
  else "\(if ($path | length) == 0 then "value" else ($path | map(tostring) | join(".")) end): file \($want | tojson), server \($got | tojson)"
  end;
diff($want; $got; [])'

# --- Output --------------------------------------------------------------------------------

pass=0
fail=0
ok() {
	pass=$((pass + 1))
	printf 'PASS  %s\n' "$1"
}
bad() {
	fail=$((fail + 1))
	printf 'FAIL  %s\n' "$1"
	if [ -n "${2:-}" ]; then printf '%s\n' "$2" | sed 's/^/      /'; fi
}
info() { printf 'INFO  %s\n' "$1"; }
die() {
	printf 'ERROR %s\n' "$1" >&2
	exit 2
}
rel() { printf '%s' "${1#"$REPO_ROOT"/}"; }

# --- Images and containers -----------------------------------------------------------------

# image_ref repo:tag -> the pinned repo:tag@sha256 line from tests/kind/images.txt.
image_ref() {
	local ref
	ref=$(awk -F@ -v want="$1" '$1 == want { print; exit }' "$IMAGES_FILE")
	[ -n "$ref" ] || die "$1 is not pinned in $(rel "$IMAGES_FILE")"
	printf '%s' "$ref"
}
command -v docker >/dev/null || die "docker not found"
[ -f "$IMAGES_FILE" ] || die "missing $IMAGES_FILE"
NATS_IMAGE=${NATS_IMAGE:-$(image_ref nats:2.15.0-alpine)}
NATS_BOX_IMAGE=${NATS_BOX_IMAGE:-$(image_ref natsio/nats-box:0.20.0)}
QDRANT_IMAGE=${QDRANT_IMAGE:-$(image_ref qdrant/qdrant:v1.19.2)}

suffix=$(od -An -N4 -tx1 /dev/urandom | tr -d ' \n')
PREFIX="af-c3check-$suffix"
NET="$PREFIX-net"
NATS_C="$PREFIX-nats"
QDRANT_C="$PREFIX-qdrant"
BOX_C="$PREFIX-box"
QDRANT_URL="http://$QDRANT_C:6333"

cleanup() {
	local rc=$?
	docker rm -f "$NATS_C" "$QDRANT_C" "$BOX_C" >/dev/null 2>&1 || true
	docker network rm "$NET" >/dev/null 2>&1 || true
	if [ -n "$(docker ps -aq --filter "label=io.affogato.check-config=$suffix")" ]; then
		printf 'WARN  containers labelled io.affogato.check-config=%s are still present\n' "$suffix" >&2
	else
		printf 'INFO  cleanup: containers and network %s-* removed\n' "$PREFIX"
	fi
	exit "$rc"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# box runs a command in the toolbox; boxi also forwards this script's stdin.
box() { docker exec "$BOX_C" "$@"; }
boxi() { docker exec -i "$BOX_C" "$@"; }
# bjq runs jq in the toolbox on stdin.
bjq() { boxi jq "$@"; }
# put_file <path in box>: write stdin to that file in the toolbox.
put_file() { boxi sh -c 'cat > "$1"' sh "$1"; }
# qd METHOD PATH [file in box]: Qdrant REST call; prints the body, fails on HTTP >= 400.
qd() {
	if [ $# -ge 3 ]; then
		box curl -sS --fail-with-body -X "$1" -H 'Content-Type: application/json' \
			--data-binary "@$3" "$QDRANT_URL$2"
	else
		box curl -sS --fail-with-body -X "$1" "$QDRANT_URL$2"
	fi
}
# twice CMD...: run a toolbox command twice (create, then idempotent re-apply).
twice() {
	local out n
	for n in 1 2; do
		if ! out=$(box "$@" 2>&1); then
			printf 'apply %s: %s\n' "$n" "$out"
			return 1
		fi
	done
}
wait_for() {
	local what=$1 i=0
	shift
	until "$@" >/dev/null 2>&1; do
		i=$((i + 1))
		[ "$i" -lt 60 ] || die "$what not ready after 30 s"
		sleep 0.5
	done
}

# compare LABEL WANT_JSON GOT_JSON PASS_DETAIL
compare() {
	local diffs
	if ! diffs=$(box jq -nr --argjson want "$2" --argjson got "$3" "$DIFF_JQ" 2>&1); then
		bad "$1: could not compare" "$diffs"
	elif [ -z "$diffs" ]; then
		ok "$1: $4"
	else
		bad "$1: server read-back differs from the file" "$diffs"
	fi
}

# --- Checks --------------------------------------------------------------------------------

check_files_present() {
	local missing="" n
	for n in $REQUIRED_STREAMS; do [ -f "$CONTRACTS_DIR/nats/streams/$n.json" ] || missing="$missing nats/streams/$n.json"; done
	for n in $REQUIRED_CONSUMERS; do [ -f "$CONTRACTS_DIR/nats/consumers/$n.json" ] || missing="$missing nats/consumers/$n.json"; done
	for n in $REQUIRED_KV; do [ -f "$CONTRACTS_DIR/nats/kv/$n.json" ] || missing="$missing nats/kv/$n.json"; done
	for n in $REQUIRED_COLLECTIONS; do [ -f "$CONTRACTS_DIR/qdrant/$n.json" ] || missing="$missing qdrant/$n.json"; done
	[ -f "$CONTRACTS_DIR/qdrant/aliases.json" ] || missing="$missing qdrant/aliases.json"
	if [ -z "$missing" ]; then
		ok "contract files present: streams ($REQUIRED_STREAMS), consumers ($REQUIRED_CONSUMERS), kv ($REQUIRED_KV), collection ($REQUIRED_COLLECTIONS), aliases.json"
	else
		bad "contract files missing:$missing"
	fi
}

# check_replicas LABEL FILE FIELD: the file must carry the production replica count.
check_replicas() {
	local r
	r=$(bjq -r ".$3" <"$2")
	if [ "$r" = "$FILE_REPLICAS" ]; then
		ok "$1: $3 $FILE_REPLICAS in the file (production value)"
	else
		bad "$1: $3 is $r in the file, want $FILE_REPLICAS (production; kind overrides)"
	fi
}

check_streams() {
	local f name label want got out
	for f in "$CONTRACTS_DIR"/nats/streams/*.json; do
		[ -e "$f" ] || continue
		name=$(bjq -r .name <"$f")
		label="stream $name ($(rel "$f"))"
		if [ "$name.json" != "$(basename "$f")" ]; then
			bad "$label: name does not match the file name"
			continue
		fi
		check_replicas "$label" "$f" num_replicas
		want=$(bjq -c --argjson r "$APPLY_REPLICAS" '.num_replicas = $r' <"$f")
		printf '%s' "$want" | put_file "/work/stream-$name.json"
		if ! out=$(twice nats stream add --config "/work/stream-$name.json"); then
			bad "$label: nats stream add failed" "$out"
			continue
		fi
		got=$(box nats stream info "$name" --json | bjq -c .config)
		compare "$label" "$want" "$got" \
			"created, re-applied, $(printf '%s' "$want" | bjq 'length') fields read back equal (num_replicas applied as $APPLY_REPLICAS)"
	done
}

check_consumers() {
	local f name stream label want got out
	for f in "$CONTRACTS_DIR"/nats/consumers/*.json; do
		[ -e "$f" ] || continue
		name=$(bjq -r .durable_name <"$f")
		stream=$(bjq -r '.metadata["affogato.stream"] // ""' <"$f")
		label="consumer $stream/$name ($(rel "$f"))"
		if [ "$name.json" != "$(basename "$f")" ]; then
			bad "$label: durable_name does not match the file name"
			continue
		fi
		if [ ! -f "$CONTRACTS_DIR/nats/streams/$stream.json" ]; then
			bad "$label: metadata.\"affogato.stream\" must name a stream in nats/streams/"
			continue
		fi
		want=$(bjq -c . <"$f")
		printf '%s' "$want" | put_file "/work/consumer-$name.json"
		if ! out=$(twice nats consumer add "$stream" --config "/work/consumer-$name.json"); then
			bad "$label: nats consumer add failed" "$out"
			continue
		fi
		got=$(box nats consumer info "$stream" "$name" --json | bjq -c .config)
		compare "$label" "$want" "$got" \
			"created, re-applied, $(printf '%s' "$want" | bjq 'length') fields read back equal"
	done
}

# The nats CLI has no --config for KV, so the KeyValueConfig file is turned into flags and
# read back from the backing stream KV_<bucket>, mapped to KeyValueConfig field names the
# same way nats.go does (history = max_msgs_per_subject, max_value_size = max_msg_size).
check_kv() {
	local f bucket label want got out unknown a
	local -a flags
	for f in "$CONTRACTS_DIR"/nats/kv/*.json; do
		[ -e "$f" ] || continue
		bucket=$(bjq -r .bucket <"$f")
		label="kv $bucket ($(rel "$f"))"
		if [ "$bucket.json" != "$(basename "$f")" ]; then
			bad "$label: bucket does not match the file name"
			continue
		fi
		unknown=$(bjq -r 'keys - ["bucket","description","history","max_value_size","max_bytes","ttl","storage","replicas","compression"] | join(" ")' <"$f")
		if [ -n "$unknown" ]; then
			bad "$label: fields this check cannot apply: $unknown"
			continue
		fi
		check_replicas "$label" "$f" replicas
		want=$(bjq -c --argjson r "$APPLY_REPLICAS" '.replicas = $r' <"$f")
		flags=()
		while IFS= read -r a; do flags+=("$a"); done < <(printf '%s' "$want" | bjq -r '
			(if has("description") then "--description=\(.description)" else empty end),
			(if has("history") then "--history=\(.history)" else empty end),
			(if has("max_value_size") then "--max-value-size=\(.max_value_size)" else empty end),
			(if has("max_bytes") then "--max-bucket-size=\(.max_bytes)" else empty end),
			(if has("ttl") then "--ttl=\(.ttl / 1000000000 | floor)s" else empty end),
			(if has("storage") then "--storage=\(.storage)" else empty end),
			(if has("replicas") then "--replicas=\(.replicas)" else empty end),
			(if .compression == true then "--compress" else empty end)')
		if ! out=$(twice nats kv add "$bucket" "${flags[@]}"); then
			bad "$label: nats kv add failed" "$out"
			continue
		fi
		got=$(box nats stream info "KV_$bucket" --json | bjq -c '.config | {
			bucket: (.name | ltrimstr("KV_")), description, history: .max_msgs_per_subject,
			max_value_size: .max_msg_size, max_bytes, ttl: .max_age, storage,
			replicas: .num_replicas, compression: (.compression == "s2")}')
		compare "$label" "$want" "$got" \
			"created, re-applied, $(printf '%s' "$want" | bjq 'length') fields read back equal via stream KV_$bucket (replicas applied as $APPLY_REPLICAS)"
	done
}

# Idempotent apply, as the H1 init Job does it: create the collection only if it does not
# exist (a second PUT is a 409), then PUT every payload index (repeatable).
apply_collection() {
	# Explicit returns: errexit is not inherited by the command substitution this runs in.
	local name=$1 n=$2 i=0 exists out
	exists=$(qd GET "/collections/$name/exists" | bjq -r .result.exists) || return 1
	if [ "$exists" != true ]; then
		out=$(qd PUT "/collections/$name" "/work/coll-$name.json" 2>&1) || {
			printf 'create: %s\n' "$out"
			return 1
		}
	fi
	while [ "$i" -lt "$n" ]; do
		out=$(qd PUT "/collections/$name/index?wait=true" "/work/coll-$name-idx-$i.json" 2>&1) || {
			printf 'payload index %s: %s\n' "$i" "$out"
			return 1
		}
		i=$((i + 1))
	done
}

check_collections() {
	local f name label want got out n i
	for f in "$CONTRACTS_DIR"/qdrant/*.json; do
		[ -e "$f" ] || continue
		[ "$(basename "$f")" != aliases.json ] || continue
		name=$(bjq -r .collection_name <"$f")
		label="collection $name ($(rel "$f"))"
		if [ "$name.json" != "$(basename "$f")" ]; then
			bad "$label: collection_name does not match the file name"
			continue
		fi
		bjq -c .collection <"$f" | put_file "/work/coll-$name.json"
		n=$(bjq '.payload_indexes | length' <"$f")
		i=0
		while [ "$i" -lt "$n" ]; do
			bjq -c ".payload_indexes[$i]" <"$f" | put_file "/work/coll-$name-idx-$i.json"
			i=$((i + 1))
		done
		if ! out=$(apply_collection "$name" "$n" 2>&1 && apply_collection "$name" "$n" 2>&1); then
			bad "$label: apply failed" "$out"
			continue
		fi
		want=$(bjq -c '{collection, payload_indexes: (.payload_indexes | sort_by(.field_name))}' <"$f")
		# Read-back: config.params plus the sections that the create body names differently
		# (optimizers_config is optimizer_config in the response); payload_schema as index list.
		got=$(qd GET "/collections/$name" | bjq -c '.result | {
			collection: (.config.params + {
				hnsw_config: .config.hnsw_config, wal_config: .config.wal_config,
				optimizers_config: .config.optimizer_config,
				quantization_config: .config.quantization_config, metadata: .config.metadata}),
			payload_indexes: (.payload_schema | to_entries
				| map({field_name: .key, field_schema: .value.data_type}) | sort_by(.field_name))}')
		compare "$label" "$want" "$got" \
			"created, re-applied, vectors + on_disk_payload + metadata + $n payload indexes read back equal"
		info "$name server defaults kept: $(qd GET "/collections/$name" | bjq -c '.result.config | {hnsw: .hnsw_config | {m, ef_construct, full_scan_threshold}, shard_number: .params.shard_number, replication_factor: .params.replication_factor}')"
	done
}

check_aliases() {
	local f label want got out a target
	f="$CONTRACTS_DIR/qdrant/aliases.json"
	label="aliases ($(rel "$f"))"
	[ -f "$f" ] || return 0
	bjq -c . <"$f" | put_file /work/aliases.json
	if ! out=$(qd POST /collections/aliases /work/aliases.json 2>&1 && qd POST /collections/aliases /work/aliases.json 2>&1); then
		bad "$label: apply failed" "$out"
		return 0
	fi
	want=$(bjq -c '[.actions[] | .create_alias // empty | {alias_name, collection_name}] | sort_by(.alias_name)' <"$f")
	got=$(qd GET /aliases | bjq -c --argjson want "$want" '[.result.aliases[]
		| select(.alias_name as $n | $want | map(.alias_name) | index($n))
		| {alias_name, collection_name}] | sort_by(.alias_name)')
	compare "$label" "$want" "$got" \
		"created, re-applied, read back equal: $(printf '%s' "$want" | bjq -r 'map("\(.alias_name) -> \(.collection_name)") | join(", ")')"
	for a in $REQUIRED_ALIASES; do
		target=$(printf '%s' "$want" | bjq -r --arg a "$a" '.[] | select(.alias_name == $a) | .collection_name')
		if [ -z "$target" ] || [ ! -f "$CONTRACTS_DIR/qdrant/$target.json" ]; then
			bad "alias $a: must point at a collection defined in contracts/qdrant/ (got '${target}')"
			continue
		fi
		# Resolving through the alias must give that collection's vector params and model.
		compare "alias $a" \
			"$(bjq -c '{vectors: .collection.vectors, embedding_model: .collection.metadata.embedding_model}' <"$CONTRACTS_DIR/qdrant/$target.json")" \
			"$(qd GET "/collections/$a" | bjq -c '{vectors: .result.config.params.vectors, embedding_model: .result.config.metadata.embedding_model}')" \
			"resolves to $target (vector params and embedding_model read through the alias)"
	done
}

# Feature 7: every stream keeps 7 days, and the DLQ age is shorter than the af-raw expiry.
check_retention() {
	local s file_age server_age raw_ns
	for s in $REQUIRED_STREAMS; do
		file_age=$(bjq -r .max_age <"$CONTRACTS_DIR/nats/streams/$s.json")
		server_age=$(box nats stream info "$s" --json | bjq -r .config.max_age)
		if [ "$file_age" = "$SEVEN_DAYS_NS" ] && [ "$server_age" = "$SEVEN_DAYS_NS" ]; then
			ok "retention (Feature 7): $s max_age 7d in file and on server"
		else
			bad "retention (Feature 7): $s max_age file $file_age ns, server $server_age ns, want $SEVEN_DAYS_NS (7d)"
		fi
	done
	raw_ns=$((AF_RAW_EXPIRY_DAYS * 86400 * 1000000000))
	server_age=$(box nats stream info VIDEO_DLQ --json | bjq -r .config.max_age)
	if [ "$server_age" -lt "$raw_ns" ]; then
		ok "retention (Feature 7): VIDEO_DLQ max_age $((server_age / 86400000000000))d < planned af-raw expiry ${AF_RAW_EXPIRY_DAYS}d (H1)"
	else
		bad "retention (Feature 7): VIDEO_DLQ max_age $server_age ns must be shorter than the af-raw expiry ${AF_RAW_EXPIRY_DAYS}d"
	fi
}

# Functional probe: a consumer that exhausts max_deliver must leave its MAX_DELIVERIES
# advisory in AF_ADVISORIES (the server accepts any subject, so only this proves it is right).
check_advisory_capture() {
	local subject='$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES.VIDEO_EVENTS.c3-probe' got="" i=0
	if ! box nats pub video.chunks.uploaded.c3-probe probe >/dev/null 2>&1 ||
		! box nats consumer add VIDEO_EVENTS c3-probe --pull --ack explicit --wait 1s --max-deliver 1 \
			--deliver all --filter video.chunks.uploaded.c3-probe --defaults >/dev/null 2>&1 ||
		! box nats consumer next VIDEO_EVENTS c3-probe --no-ack --count 1 >/dev/null 2>&1; then
		bad "stream AF_ADVISORIES: could not set up the MAX_DELIVERIES probe"
		return 0
	fi
	# Fetching again after ack_wait (1 s) makes the server hit max_deliver and advise.
	while [ "$i" -lt 10 ] && [ "$got" != "$subject" ]; do
		box nats consumer next VIDEO_EVENTS c3-probe --count 1 --wait 1s >/dev/null 2>&1 || true
		got=$(box nats stream get AF_ADVISORIES --last-for "$subject" --json 2>/dev/null | bjq -r .subject 2>/dev/null || true)
		i=$((i + 1))
	done
	box nats consumer rm VIDEO_EVENTS c3-probe --force >/dev/null 2>&1 || true
	if [ "$got" = "$subject" ]; then
		ok "stream AF_ADVISORIES captured $subject"
	else
		bad "stream AF_ADVISORIES did not capture a MAX_DELIVERIES advisory for a probe consumer"
	fi
}

# --- Main ----------------------------------------------------------------------------------

info "images: $NATS_IMAGE, $NATS_BOX_IMAGE, $QDRANT_IMAGE"
info "containers: $PREFIX-{nats,qdrant,box} on network $NET (no host ports)"
docker network create --label "io.affogato.check-config=$suffix" "$NET" >/dev/null
docker run -d --name "$NATS_C" --network "$NET" --label "io.affogato.check-config=$suffix" \
	"$NATS_IMAGE" -js >/dev/null
docker run -d --name "$QDRANT_C" --network "$NET" --label "io.affogato.check-config=$suffix" \
	"$QDRANT_IMAGE" >/dev/null
docker run -d --name "$BOX_C" --network "$NET" --label "io.affogato.check-config=$suffix" \
	-e "NATS_URL=nats://$NATS_C:4222" "$NATS_BOX_IMAGE" sleep 3600 >/dev/null
box mkdir -p /work
wait_for "NATS JetStream" box nats account info
wait_for "Qdrant" box curl -fsS "$QDRANT_URL/readyz"
info "servers: $(docker exec "$NATS_C" nats-server --version), qdrant $(box curl -fsS "$QDRANT_URL/" | bjq -r .version)"

check_files_present
check_streams
check_consumers
check_kv
check_collections
check_aliases
check_retention
check_advisory_capture

printf 'SUMMARY: %d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
