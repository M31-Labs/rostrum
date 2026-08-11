.PHONY: dev check test build smoke size-budget perf-budget release-check check-gosx

GOSX ?= gosx
GOSX_VERSION ?= 0.38.1
PERF_URLS ?= http://localhost:8080/ http://localhost:8080/organizer http://localhost:8080/organizer/agenda http://localhost:8080/organizer/portal http://localhost:8080/public/m31-systems-forum-2026/agenda
SMOKE_URL ?=

dev:
	DEMO_MODE=memory go run .

test:
	go list ./... | grep -v '/dist' | xargs go test
	go list ./... | grep -v '/dist' | xargs go test -race

check-gosx:
	@actual="$$($(GOSX) version 2>/dev/null || true)"; \
	if [ "$$actual" != "gosx v$(GOSX_VERSION)" ]; then \
		echo "Rostrum requires gosx v$(GOSX_VERSION); found '$$actual'."; \
		echo "Install it with: go install m31labs.dev/gosx/cmd/gosx@v$(GOSX_VERSION)"; \
		exit 1; \
	fi

check: check-gosx
	test -z "$$(gofmt -l $$(find . -name '*.go' -not -path './dist/*' -not -path './build/*'))"
	$(GOSX) fmt --check app
	for policy_file in rules/*.arb; do arbiter check "$$policy_file"; done
	go list ./... | grep -v '/dist' | xargs go vet
	go list ./... | grep -v '/dist' | xargs go test
	go list ./... | grep -v '/dist' | xargs go test -race

build: check-gosx
	rm -rf -- dist
	CGO_ENABLED=0 $(GOSX) build --prod .
	# GoSX produces the browser assets, but its host-server build does not yet
	# apply production linker flags. Replace that equivalent binary with the
	# reproducible stripped server artifact used by the deployment bundle.
	CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o dist/server/app .
	find dist/assets/runtime -type f -name '*.map' -delete
	find dist/app -type f -name '*.go' -delete

smoke:
	scripts/smoke.sh $(SMOKE_URL)

size-budget: build
	GOSX=$(GOSX) go run ./cmd/sizecheck -root .

perf-budget:
	$(GOSX) perf --mobile pixel7 --throttle 4 --coverage --budget perf-budget.json $(PERF_URLS)

release-check: check size-budget
