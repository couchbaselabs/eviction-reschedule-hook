# Image URL to use all building/pushing image targets
DOCKER_IMAGE := couchbase-reschedule-hook
DOCKER_USER := 
DOCKER_TAG := latest

# Go environment variables
GOPATH := $(shell go env GOPATH)
GOBIN := $(if $(GOPATH),$(GOPATH)/bin,$(HOME)/go/bin)
GOOS := linux
GOARCH := amd64

.PHONY: lint
lint:
	@echo "Installing golangci-lint..."
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(GOBIN)
	@echo "Running golangci-lint..."
	$(GOBIN)/golangci-lint run --timeout=15m ./pkg/... ./cmd/...


.PHONY: build
build: ## Build the binary
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o bin/couchbase-reschedule-hook cmd/main.go

.PHONY: docker-build
docker-build: build ## Build docker image with the manager.
	docker build -t ${DOCKER_USER}/${DOCKER_IMAGE}:${DOCKER_TAG} -f docker/Dockerfile .

.PHONY: kind-images
kind-images: docker-build ## Build and load docker image into kind
	kind load docker-image ${DOCKER_USER}/${DOCKER_IMAGE}:${DOCKER_TAG}

.PHONY: images-public
images-public: docker-build
	docker push ${DOCKER_USER}/${DOCKER_IMAGE}:${DOCKER_TAG}

.PHONY: images-clean
images-clean:
	docker rmi -f ${DOCKER_USER}/${DOCKER_IMAGE}:${DOCKER_TAG}