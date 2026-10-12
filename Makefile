# Affogato root targets.
#
#   make contracts-gen     regenerate gen/{go,rust,python} from proto/ (buf generate)
#   make contracts-check   what CI checks for the contracts: buf format + lint, regenerate and
#                          require gen/ to be unchanged, then build the generated code in Go,
#                          Rust and Python
#
# Tools default to containers or host binaries and can be overridden, e.g. BUF=buf GO=go.
# A host buf must be the pinned version (BUF_VERSION) or format/generate output may differ.

BUF_VERSION ?= 1.73.0
BUF         ?= docker run --rm --user "$$(id -u):$$(id -g)" -e BUF_CACHE_DIR=/tmp/buf-cache \
               -v "$(CURDIR):/workspace" -w /workspace bufbuild/buf:$(BUF_VERSION)
# Host go if present, else the golang container wrapper.
GO          ?= $(if $(shell command -v go 2>/dev/null),go,$(CURDIR)/tests/go-docker.sh)
CARGO       ?= cargo
PYTHON      ?= python3
# Throw-away venv for the Python import check (git-ignored).
PY_VENV     ?= $(CURDIR)/.venv

# Every generated file; removed before `buf generate` so files of deleted protos do not linger.
GEN_CLEAN = find gen/go gen/python/src -type f \( -name '*.pb.go' -o -name '*_pb2.py' -o -name '*_pb2.pyi' \) -delete; \
            rm -rf gen/rust/src

.PHONY: contracts-gen contracts-check

contracts-gen:
	@mkdir -p gen/go gen/python/src
	$(GEN_CLEAN)
	$(BUF) generate

contracts-check:
	$(BUF) format --diff --exit-code
	$(BUF) lint
	$(MAKE) --no-print-directory contracts-gen
	git diff --exit-code -- gen/
	@untracked="$$(git ls-files --others --exclude-standard -- gen/)"; \
	  if [ -n "$$untracked" ]; then echo "untracked generated files (commit them):"; echo "$$untracked"; exit 1; fi
	cd gen/go && $(GO) build ./... && $(GO) vet ./...
	$(CARGO) build --locked --manifest-path gen/rust/Cargo.toml
	$(PYTHON) -m venv $(PY_VENV)
	$(PY_VENV)/bin/pip install --quiet --disable-pip-version-check --force-reinstall ./gen/python
	$(PY_VENV)/bin/python -I -c "import affogato.events.v1.chunk_events_pb2"
	$(PY_VENV)/bin/python -I -c "import affogato.common.v1.common_pb2, affogato.events.v1.chunk_events_pb2, \
	  affogato.events.v1.video_events_pb2, affogato.dlq.v1.dead_letter_pb2, affogato.state.v1.hls_index_pb2, \
	  affogato.artifacts.v1.transcript_pb2, affogato.embed.v1.embed_pb2; print('python: all 7 modules import')"
	@echo "contracts-check: OK"
