# syntax=docker/dockerfile:1
# Acceptance runner image (B4: tests/acceptance packaged as an image, run as a Job on kind).
# Build from the repository root so gen/go can be added later via a go.mod `replace`:
#   docker build -f tests/acceptance/Dockerfile -t affogato/acceptance:dev .

ARG GO_VERSION=1.26
ARG DEBIAN=bookworm
# Keep in sync with FFMPEG_IMAGE in tests/ffmpeg-docker.sh.
ARG FFMPEG_IMAGE=linuxserver/ffmpeg:version-7.1.1-cli@sha256:aea59a11c54291ac456bb2d67000445e5a8994f70bc3d96cdc29f022fbbf89fb

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
# Same pinned ffmpeg as tests/ffmpeg-docker.sh, so in-cluster fixtures match local ones.
# make_fixtures.sh runs verify_fixtures.sh, so a bad fixture fails the build.
FROM ${FFMPEG_IMAGE} AS fixtures
COPY tests/fixtures/ /fixtures-src/
RUN AF_IN_FFMPEG_CONTAINER=1 bash /fixtures-src/make_fixtures.sh /fixtures

# ---- kubectl (checksum-verified) for fault injection ----
FROM debian:${DEBIAN}-slim AS kubectl
ARG KUBECTL_VERSION=v1.36.1
ARG TARGETARCH
RUN apt-get update && apt-get install -y --no-install-recommends curl ca-certificates \
    && arch="${TARGETARCH:-$(dpkg --print-architecture)}" \
    && curl -fsSLo /kubectl "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${arch}/kubectl" \
    && echo "$(curl -fsSL "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${arch}/kubectl.sha256")  /kubectl" | sha256sum -c - \
    && chmod 0755 /kubectl

# TODO(developer): once the images exist, copy the Producer CLI and afctl into the runner:
# FROM affogato/producer:dev AS producer
# FROM affogato/afctl:dev AS afctl
# ...and below:  COPY --from=producer /af-producer /usr/local/bin/af-producer
#                COPY --from=afctl /afctl /usr/local/bin/afctl

# ---- runtime ----
# The pinned ffmpeg image already has ffprobe (remux verification, Feature 1) and CA
# certificates, so the runner needs no apt step and uses the same ffprobe as the fixtures.
FROM ${FFMPEG_IMAGE}
COPY --from=kubectl /kubectl /usr/local/bin/kubectl
COPY --from=fixtures /fixtures /fixtures
COPY --from=build /out/acceptance.test /acceptance/acceptance.test
COPY tests/acceptance/features /acceptance/features
WORKDIR /acceptance
ENV AF_FIXTURES_DIR=/fixtures NO_COLOR=1 GODOG_TAGS=~@load
USER 65532:65532
ENTRYPOINT ["/acceptance/acceptance.test"]
CMD ["-test.run", "^TestFeatures$", "-test.v"]
