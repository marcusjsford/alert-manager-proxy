VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
IMAGE   ?= alertmanager-proxy
LINT_IMAGE ?= golangci/golangci-lint:v2.14.0
AM_PORT ?= 19093
COMPOSE = docker compose -f test/integration/docker-compose.yml -p amp-integration

.PHONY: build test lint integration docker clean

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o bin/alertmanager-proxy ./cmd/alertmanager-proxy

test:
	go vet ./...
	go test -race ./...

lint:
	docker run --rm -v "$(CURDIR)":/src -v "$$(go env GOMODCACHE)":/go/pkg/mod -w /src $(LINT_IMAGE) golangci-lint run ./...

# Pass-through against a real Alertmanager (version pinned in the compose file).
integration:
	AM_PORT=$(AM_PORT) $(COMPOSE) up -d --wait
	AMP_IT_UPSTREAM_URL=http://127.0.0.1:$(AM_PORT) go test -tags integration -count=1 -v ./test/integration/; \
		status=$$?; $(COMPOSE) down -v; exit $$status

docker:
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE):$(VERSION) .

clean:
	rm -rf bin
