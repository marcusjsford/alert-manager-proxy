VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
IMAGE   ?= alertmanager-proxy
LINT_IMAGE ?= golangci/golangci-lint:v2.14.0
AM_VERSION ?= v0.34.1
REFERENCE_VERSION ?= v0.26.0
TARGET_VERSION ?= v0.34.1
AM_PORT ?= 19093
COMPOSE = docker compose -f test/integration/docker-compose.yml -p amp-integration
CONTRACT_COMPOSE = docker compose -f test/contract/docker-compose.yml -p amp-contract
LAB_COMPOSE = docker compose -f test/lab/docker-compose.yml -p amp-lab
TELEMETRY_COMPOSE = docker compose -f test/telemetry/docker-compose.yml -p amp-telemetry
FANOUT_COMPOSE = docker compose -f test/fanout/docker-compose.yml -p amp-fanout
CONTRACT = go run ./test/contract/cmd/contract
FIXTURES = test/contract/fixtures/v0.26.0
PENDING ?= test/contract/pending.txt
CONTRACT_FLAGS ?=
V1_CLIENT_ARGS ?= --url http://127.0.0.1:19391 --url http://127.0.0.1:19392 --url http://127.0.0.1:19393 --receiver http://127.0.0.1:19280

.PHONY: build test lint integration integration-v026 contract strict-contract release-check contract-record contract-determinism v1-client-check v1-client-syntax lab lab-check lab-down telemetry telemetry-check telemetry-down fanout fanout-check fanout-down dashboards-check docker ci clean

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o bin/alertmanager-proxy ./cmd/alertmanager-proxy

test:
	go vet ./...
	go test -race ./...

lint:
	docker run --rm -v "$(CURDIR)":/src -v "$$(go env GOMODCACHE)":/go/pkg/mod -w /src $(LINT_IMAGE) golangci-lint run ./...

# Pass-through against a real Alertmanager (version pinned by AM_VERSION).
integration:
	AM_VERSION=$(AM_VERSION) AM_PORT=$(AM_PORT) $(COMPOSE) up -d --wait
	AM_VERSION=$(AM_VERSION) AMP_IT_UPSTREAM_URL=http://127.0.0.1:$(AM_PORT) go test -tags integration -count=1 -v ./test/integration/; \
		status=$$?; $(COMPOSE) down -v; exit $$status

integration-v026:
	$(MAKE) integration AM_VERSION=$(REFERENCE_VERSION)

# v1 contract: replay the recorded v0.26.0 fixtures against proxy -> current
# Alertmanager. Per-endpoint table; pending endpoints (test/contract/pending.txt)
# are expected failures. CONTRACT_FLAGS=-strict makes release mode require an
# empty pending list.
contract:
	AM_VERSION=$(AM_VERSION) $(CONTRACT_COMPOSE) --profile verify up -d --build --force-recreate
	$(CONTRACT) verify $(CONTRACT_FLAGS) -target http://127.0.0.1:19195 -fixtures $(FIXTURES) -pending $(PENDING); \
		status=$$?; $(CONTRACT_COMPOSE) --profile verify down -v; exit $$status

# Release gate: both the initial deployment version and the selected upgrade
# target must pass every fixture with no exemptions.
strict-contract:
	$(MAKE) contract CONTRACT_FLAGS=-strict AM_VERSION=$(REFERENCE_VERSION)
	$(MAKE) contract CONTRACT_FLAGS=-strict AM_VERSION=$(TARGET_VERSION)

release-check: strict-contract

# Legacy Python v1 client probe; requires the manual lab or explicit --url args.
v1-client-check:
	python3 test/clients/v1_client.py $(V1_CLIENT_ARGS)

v1-client-syntax:
	python3 -c 'import ast, pathlib; ast.parse(pathlib.Path("test/clients/v1_client.py").read_text())'

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

telemetry:
	$(TELEMETRY_COMPOSE) up -d --build --wait --wait-timeout 90

telemetry-check:
	go test -race -tags telemetry -count=1 -timeout=3m -v ./test/telemetry/

telemetry-down:
	$(TELEMETRY_COMPOSE) down -v

# Three-AM/three-proxy partial-fanout detection stack (separate from amp-lab).
fanout:
	$(FANOUT_COMPOSE) up -d --build --wait --wait-timeout 90

fanout-check:
	go test -race -tags fanout -count=1 -timeout=2m -v ./test/fanout/

fanout-down:
	$(FANOUT_COMPOSE) down -v

# Dashboard/rules checks use the fanout stack for ClickHouse data.
dashboards-check:
	go test -tags dashboards -count=1 -timeout=3m -v ./test/dashboards/

rules-check:
	docker run --rm -v "$(CURDIR)/deploy/rules:/rules:ro" --entrypoint promtool prom/prometheus:v2.55.1 test rules /rules/alert_path_test.yml

docker:
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE):$(VERSION) .

# Everything the Forgejo pipeline runs, locally. The persistent manual lab is
# separate; integration and contract create and remove their own environments.
# The strict contract runs both versions and is the release gate.
ci: build test lint integration integration-v026 strict-contract docker

clean:
	rm -rf bin
