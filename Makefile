.PHONY: all
all: go-lint apex

.PHONY: apex
apex: dist/apex dist/apex-linux-arm dist/apex-linux-amd64 dist/apex-darwin-amd64 dist/apex-darwin-arm64 dist/apex-windows-amd64

COMMON_DEPS=$(wildcard ./internal/**/*.go) go.sum go.mod

APEX_DEPS=$(COMMON_DEPS) $(wildcard cmd/apex/*.go)

APISERVER_DEPS=$(COMMON_DEPS) $(wildcard cmd/apiserver/*.go)

TAG=$(shell git rev-parse HEAD)

dist:
	mkdir -p $@

dist/apex: $(APEX_DEPS) | dist
	CGO_ENABLED=0 go build -o $@ ./cmd/apex

dist/apex-linux-arm: $(APEX_DEPS) | dist
	CGO_ENABLED=0 GOOS=linux GOARCH=arm go build -o $@ ./cmd/apex

dist/apex-linux-amd64: $(APEX_DEPS) | dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $@ ./cmd/apex

dist/apex-darwin-amd64: $(APEX_DEPS) | dist
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o $@ ./cmd/apex

dist/apex-darwin-arm64: $(APEX_DEPS) | dist
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o $@ ./cmd/apex

dist/apex-windows-amd64: $(APEX_DEPS) | dist
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -o $@ ./cmd/apex

dist/apexctl: $(APEX_DEPS) | dist
	CGO_ENABLED=0 go build -o $@ ./cmd/apexctl

.PHONY: clean
clean:
	rm -rf dist

.PHONY: go-lint
go-lint: $(APEX_DEPS) $(APISERVER_DEPS)
	@if ! which golangci-lint 2>&1 >/dev/null; then \
		echo "Please install golangci-lint." ; \
		echo "See: https://golangci-lint.run/usage/install/#local-installation" ; \
		exit 1 ; \
	fi
	golangci-lint run ./...

.PHONY: gen-docs
gen-docs:
	swag init -g ./cmd/apiserver/main.go -o ./internal/docs

.PHONY: test-images
test-images:
	docker build -f Containerfile.test -t quay.io/apex/test:alpine --target alpine .
	docker build -f Containerfile.test -t quay.io/apex/test:fedora --target fedora .
	docker build -f Containerfile.test -t quay.io/apex/test:ubuntu --target ubuntu .

.PHONY: e2e
e2e: dist/apex dist/apexctl test-images
	go test -v --tags=integration ./integration-tests/...

.PHONY: unit
unit:
	go test -v ./...

.PHONY: images image-frontend image-apiserver image-apex
image-apex:
	docker build -f Containerfile.apex -t quay.io/apex/apex:$(TAG) .
	docker tag quay.io/apex/apex:$(TAG) quay.io/apex/apex:latest

image-frontend:
	docker build -f Containerfile.frontend -t quay.io/apex/frontend:$(TAG) .
	docker tag quay.io/apex/frontend:$(TAG) quay.io/apex/frontend:latest

image-apiserver:
	docker build -f Containerfile.apiserver -t quay.io/apex/apiserver:$(TAG) .
	docker tag quay.io/apex/apiserver:$(TAG) quay.io/apex/apiserver:latest

# Not build image-apex with this target, 
# because for local environment we don't need apex image,
# so save some build time.
images: image-frontend image-apiserver
