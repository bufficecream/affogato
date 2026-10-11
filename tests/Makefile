# FramePulse test entry points (Kubernetes-only acceptance; spec A10).
#
#   make unit             helper unit tests + step coverage, no infrastructure
#   make vet              go vet the acceptance module
#   make acceptance-dry   godog over ALL scenarios (incl. @load); every step must be defined,
#                         pending is OK; no cluster needed
#   make fixtures         generate media fixtures (ffmpeg)
#   make kind-up          create the kind cluster
#   make kind-down        delete it
#   make image            build the runner image
#   make acceptance       kind load image + run in-cluster Job, excluding @load
#   make load             same, only @load scenarios (dedicated load runs)
#   make acceptance-host  run from the host against port-forwarded services
#
# No local Go? Use the containerised toolchain: make unit GO="$(PWD)/go-docker.sh"

GO            ?= go
KIND          ?= kind
KUBECTL       ?= kubectl
DOCKER        ?= docker
KIND_CLUSTER  ?= framepulse
KIND_NODE_IMAGE ?=
IMAGE         ?= framepulse/acceptance:dev
NAMESPACE     ?= framepulse
GODOG_STRICT  ?= false
ACC           := acceptance
ROOT          := $(abspath $(CURDIR)/..)

export NAMESPACE GODOG_STRICT KUBECTL

.PHONY: unit vet acceptance-dry fixtures kind-up kind-down image kind-load acceptance load acceptance-host

unit:
	cd $(ACC) && $(GO) test -count=1 ./internal/... ./steps/...

vet:
	cd $(ACC) && $(GO) vet ./...

acceptance-dry:
	cd $(ACC) && NO_COLOR=1 $(GO) test -count=1 -run '^TestFeatures$$' . -v \
		-args -godog.tags= -godog.format=progress

fixtures:
	./fixtures/make_fixtures.sh

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
	cd $(ACC) && FP_FIXTURES_DIR=$(ROOT)/tests/fixtures/media $(GO) test -count=1 -run '^TestFeatures$$' . -v \
		-args -godog.tags='~@load'
