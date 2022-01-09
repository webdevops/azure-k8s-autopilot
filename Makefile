PROJECT_NAME		:= azure-k8s-autopilot
GIT_TAG				:= $(shell git describe --dirty --tags --always)
GIT_COMMIT			:= $(shell git rev-parse --short HEAD)
LDFLAGS				:= -X "main.gitTag=$(GIT_TAG)" -X "main.gitCommit=$(GIT_COMMIT)" -extldflags "-static"

FIRST_GOPATH			:= $(firstword $(subst :, ,$(shell go env GOPATH)))
GOLANGCI_LINT_BIN		:= $(FIRST_GOPATH)/bin/golangci-lint

.PHONY: all
all: build

.PHONY: clean
clean:
	git clean -Xfd .

.PHONY: build
build:
	CGO_ENABLED=0 go build -a -ldflags '$(LDFLAGS)' -o $(PROJECT_NAME) .

.PHONY: vendor
vendor:
	go mod tidy
	go mod vendor
	go mod verify

.PHONY: image
image: build
	docker build -t $(PROJECT_NAME):$(GIT_TAG) .

build-push-development:
	docker build -t webdevops/$(PROJECT_NAME):development . && docker push webdevops/$(PROJECT_NAME):development

.PHONY: test
test:
	go test ./...

.PHONY: recreate-go-mod
recreate-go-mod:
	rm -f go.mod go.sum
	GO111MODULE=on go mod init github.com/webdevopos/azure-k8s-autopilot
	GO111MODULE=on go get k8s.io/client-go@v0.22.0
	GO111MODULE=on go get -u github.com/Azure/azure-sdk-for-go/...
	GO111MODULE=on go get
	GO111MODULE=on go mod vendor

.PHONY: lint
lint: $(GOLANGCI_LINT_BIN)
	$(GOLANGCI_LINT_BIN) run -E exportloopref,gofmt --timeout=10m

.PHONY: dependencies
dependencies: $(GOLANGCI_LINT_BIN)

$(GOLANGCI_LINT_BIN):
	curl -sfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh -s -- -b $(FIRST_GOPATH)/bin v1.32.2
