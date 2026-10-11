# FramePulse test entry points (Kubernetes-only acceptance; spec A10).
#
#   make unit             helper unit tests + step coverage, no infrastructure
#   make vet              go vet the acceptance module
#   make acceptance-dry   godog over ALL scenarios (incl. @load); every step must be defined,
#                         pending is OK; no cluster needed
#   make fixtures         generate + verify media fixtures (host ffmpeg, else pinned container)
#   make fixtures-verify  re-verify existing fixtures with ffprobe (no regeneration)
#   make manifests-lint   validate tests/kind manifests with kubeconform (Docker, no cluster)
#   make kind-up          create the kind cluster
#   make kind-down        delete it
#   make image            build the runner image
#   make acceptance       kind load image + run in-cluster Job, excluding @load
#   make load             same, only @load scenarios (dedicated load runs)
#   make acceptance-host  run from the host against port-forwarded services
#
# No local Go? Use the containerised toolchain: make unit GO="$(PWD)/go-docker.sh"
# No local ffmpeg? `make fixtures` falls back to the pinned container automatically;
# FFMPEG="$(PWD)/ffmpeg-docker.sh" runs one container per ffmpeg call instead,
# FIXTURES_DOCKER=1 forces the container even when a host ffmpeg exists.

GO            ?= go
KIND          ?= kind
KUBECTL       ?= kubectl
DOCKER        ?= docker
KIND_CLUSTER  ?= framepulse
KIND_NODE_IMAGE ?=
IMAGE         ?= framepulse/acceptance:dev
NAMESPACE     ?= framepulse
GODOG_STRICT  ?= false
FFMPEG        ?=
FFPROBE       ?=
FIXTURES_DOCKER ?= 0
FIXTURES_DIR  ?= $(CURDIR)/fixtures/media
KUBECONFORM_IMAGE ?= ghcr.io/yannh/kubeconform:v0.6.7
KUBE_SCHEMA_VERSION ?= 1.33.0
ACC           := acceptance
ROOT          := $(abspath $(CURDIR)/..)

export NAMESPACE GODOG_STRICT KUBECTL

.PHONY: unit vet acceptance-dry fixtures fixtures-verify manifests-lint kind-up kind-down image kind-load acceptance load acceptance-host

unit:
	cd $(ACC) && $(GO) test -count=1 ./internal/... ./steps/...

vet:
	cd $(ACC) && $(GO) vet ./...

acceptance-dry:
	cd $(ACC) && NO_COLOR=1 $(GO) test -count=1 -run '^TestFeatures$$' . -v \
		-args -godog.tags= -godog.format=progress

fixtures:
	FFMPEG="$(FFMPEG)" FFPROBE="$(FFPROBE)" FIXTURES_DOCKER=$(FIXTURES_DOCKER) ./fixtures/make_fixtures.sh "$(FIXTURES_DIR)"

fixtures-verify:
	FFMPEG="$(FFMPEG)" FFPROBE="$(FFPROBE)" FIXTURES_DOCKER=$(FIXTURES_DOCKER) ./fixtures/verify_fixtures.sh "$(FIXTURES_DIR)"

# The Job template's __PLACEHOLDERS__ are substituted the way kind/run-job.sh does. The kind
# Cluster config has no published schema and is reported as skipped. kubeconform downloads
# the Kubernetes JSON schemas, so this needs network access but no cluster.
manifests-lint:
	@tmp=$$(mktemp -d) && trap 'rm -rf "$$tmp"' EXIT && \
	sed -e 's|__JOB_NAME__|fp-acceptance|g' -e 's|__IMAGE__|$(IMAGE)|g' \
	    -e 's|__GODOG_TAGS__|~@load|g' -e 's|__GODOG_STRICT__|false|g' kind/acceptance-job.yaml > "$$tmp/acceptance-job.yaml" && \
	cp kind/acceptance-rbac.yaml kind/kind-config.yaml "$$tmp/" && \
	$(DOCKER) run --rm -v "$$tmp:/m:ro" $(KUBECONFORM_IMAGE) -strict -summary -verbose \
	    -kubernetes-version $(KUBE_SCHEMA_VERSION) -ignore-missing-schemas /m/acceptance-job.yaml /m/acceptance-rbac.yaml /m/kind-config.yaml

kind-up:
	$(KIND) create cluster --name $(KIND_CLUSTER) --config kind/kind-config.yaml \
		$(if $(KIND_NODE_IMAGE),--image $(KIND_NODE_IMAGE),) --wait 120s
	$(KUBECTL) create namespace $(NAMESPACE) --dry-run=client -o yaml | $(KUBECTL) apply -f -
	@echo "next: helm install framepulse deploy/helm/framepulse -n $(NAMESPACE) -f deploy/helm/framepulse/values-kind.yaml --wait"

kind-down:
	$(KIND) delete cluster --name $(KIND_CLUSTER)

image:
	$(DOCKER) build -f $(ACC)/Dockerfile -t $(IMAGE) $(ROOT)

kind-load: image
	$(KIND) load docker-image $(IMAGE) --name $(KIND_CLUSTER)

acceptance: kind-load
	./kind/run-job.sh $(IMAGE) '~@load' fp-acceptance

load: kind-load
	./kind/run-job.sh $(IMAGE) '@load' fp-acceptance-load

acceptance-host:
	@echo "requires ./kind/port-forward.sh running and FP_MINIO_ACCESS_KEY/FP_MINIO_SECRET_KEY set"
	cd $(ACC) && FP_FIXTURES_DIR=$(FIXTURES_DIR) $(GO) test -count=1 -run '^TestFeatures$$' . -v \
		-args -godog.tags='~@load'
