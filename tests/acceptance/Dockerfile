# syntax=docker/dockerfile:1
# Acceptance runner image (B4: tests/acceptance packaged as an image, run as a Job on kind).
# Build from the repository root so gen/go can be added later via a go.mod `replace`:
#   docker build -f tests/acceptance/Dockerfile -t framepulse/acceptance:dev .

ARG GO_VERSION=1.26
ARG DEBIAN=bookworm

# ---- test binary ----
FROM golang:${GO_VERSION}-${DEBIAN} AS build
WORKDIR /src/tests/acceptance
COPY tests/acceptance/go.mod tests/acceptance/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY tests/acceptance/ ./
# Helper unit tests gate the image: a broken helper never reaches the cluster.
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go vet ./... && go test ./internal/... ./steps/... && \
    CGO_ENABLED=0 go test -c -o /out/acceptance.test .

# ---- media fixtures (generated, never committed) ----
FROM debian:${DEBIAN}-slim AS fixtures
RUN apt-get update && apt-get install -y --no-install-recommends ffmpeg ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY tests/fixtures/ /fixtures-src/
RUN bash /fixtures-src/make_fixtures.sh /fixtures

# ---- kubectl (checksum-verified) for fault injection ----
FROM debian:${DEBIAN}-slim AS kubectl
ARG KUBECTL_VERSION=v1.36.1
ARG TARGETARCH
RUN apt-get update && apt-get install -y --no-install-recommends curl ca-certificates \
    && arch="${TARGETARCH:-$(dpkg --print-architecture)}" \
    && curl -fsSLo /kubectl "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${arch}/kubectl" \
    && echo "$(curl -fsSL "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${arch}/kubectl.sha256")  /kubectl" | sha256sum -c - \
    && chmod 0755 /kubectl

# TODO(developer): once the images exist, copy the Producer CLI and fpctl into the runner:
# FROM framepulse/producer:dev AS producer
# FROM framepulse/fpctl:dev AS fpctl
# ...and below:  COPY --from=producer /fp-producer /usr/local/bin/fp-producer
#                COPY --from=fpctl /fpctl /usr/local/bin/fpctl

# ---- runtime ----
FROM debian:${DEBIAN}-slim
# ffprobe: remux verification (Feature 1); ca-certificates for TLS endpoints.
RUN apt-get update && apt-get install -y --no-install-recommends ffmpeg ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=kubectl /kubectl /usr/local/bin/kubectl
COPY --from=fixtures /fixtures /fixtures
COPY --from=build /out/acceptance.test /acceptance/acceptance.test
COPY tests/acceptance/features /acceptance/features
WORKDIR /acceptance
ENV FP_FIXTURES_DIR=/fixtures NO_COLOR=1 GODOG_TAGS=~@load
USER 65532:65532
ENTRYPOINT ["/acceptance/acceptance.test"]
CMD ["-test.run", "^TestFeatures$", "-test.v"]
