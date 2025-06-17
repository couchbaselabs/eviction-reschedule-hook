DOCKER_IMAGE ?= couchbase-reschedule-hook
DOCKER_USER ?= couchbase
DOCKER_TAG ?= latest
KIND_CLUSTER_NAME ?= kind

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
	$(GOBIN)/golangci-lint run --timeout=15m ./pkg/... ./cmd/... ./test/...

.PHONY: build
build: ## Build the binary
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o bin/couchbase-reschedule-hook cmd/main.go

.PHONY: docker-build
docker-build: ## Build docker image
	docker build -t ${DOCKER_USER}/${DOCKER_IMAGE}:${DOCKER_TAG} -f docker/Dockerfile .

.PHONY: kind-image
kind-image: docker-build ## Build and load docker image into kind
	kind load docker-image --name $(KIND_CLUSTER_NAME) ${DOCKER_USER}/${DOCKER_IMAGE}:${DOCKER_TAG} 

.PHONY: public-image
public-image: docker-build ## Push docker image to docker hub
	docker push ${DOCKER_USER}/${DOCKER_IMAGE}:${DOCKER_TAG}

.PHONY: images-clean
images-clean: ## Remove docker image
	docker rmi -f ${DOCKER_USER}/${DOCKER_IMAGE}:${DOCKER_TAG}

.PHONY: test ## Run both unit and e2e tests
test: test-unit test-e2e

.PHONY: test-unit
test-unit: ## Run all unit tests
	go test -v ./pkg/reschedule/...

.PHONY: test-e2e
test-e2e: ## Run all e2e tests
	go test -v -count=1 ./test/e2e/...
