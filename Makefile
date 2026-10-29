VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
IMAGE   ?= alertmanager-proxy
LINT_IMAGE ?= golangci/golangci-lint:v2.14.0
AM_PORT ?= 19093
COMPOSE = docker compose -f test/integration/docker-compose.yml -p amp-integration
CONTRACT_COMPOSE = docker compose -f test/contract/docker-compose.yml -p amp-contract
LAB_COMPOSE = docker compose -f test/lab/docker-compose.yml -p amp-lab
CONTRACT = go run ./test/contract/cmd/contract
FIXTURES = test/contract/fixtures/v0.26.0

.PHONY: build test lint integration contract contract-record contract-determinism lab lab-check lab-down docker ci clean

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

# v1 contract: replay the recorded v0.26.0 fixtures against proxy -> current
# Alertmanager. Per-endpoint table; pending endpoints (test/contract/pending.txt)
# are expected failures.
contract:
	$(CONTRACT_COMPOSE) --profile verify up -d --build --force-recreate
	$(CONTRACT) verify -target http://127.0.0.1:19195 -fixtures $(FIXTURES); \
		status=$$?; $(CONTRACT_COMPOSE) --profile verify down -v; exit $$status

# Re-record the fixtures from a fresh Alertmanager v0.26.0.
contract-record:
	$(CONTRACT_COMPOSE) --profile record up -d --force-recreate
	$(CONTRACT) record -target http://127.0.0.1:19126 -out $(FIXTURES); \
		status=$$?; $(CONTRACT_COMPOSE) --profile record down -v; exit $$status

# Record twice from two fresh v0.26.0 instances and require identical
# normalised results: proves the normaliser and scenario aren't flaky.
contract-determinism:
	@tmp=$$(mktemp -d); \
	for run in a b; do \
		$(CONTRACT_COMPOSE) --profile record up -d --force-recreate >/dev/null 2>&1 && \
		$(CONTRACT) record -target http://127.0.0.1:19126 -out $$tmp/$$run || exit 2; \
		$(CONTRACT_COMPOSE) --profile record down -v >/dev/null 2>&1; \
	done; \
	$(CONTRACT) compare -a $$tmp/a -b $$tmp/b; status=$$?; rm -rf $$tmp; exit $$status

# Persistent, isolated development environment; separate from disposable CI rigs.
lab:
	$(LAB_COMPOSE) up -d --build --wait --wait-timeout 90

lab-check:
	go test -race -tags lab -count=1 -timeout=2m -v ./test/lab/

lab-down:
	$(LAB_COMPOSE) down -v

docker:
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE):$(VERSION) .

# Everything the Forgejo pipeline runs, locally. The persistent manual lab is
# separate; integration and contract create and remove their own environments.
ci: build test lint integration contract docker

clean:
	rm -rf bin
