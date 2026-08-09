.PHONY: dev check test build size-budget perf-budget release-check

GOSX ?= gosx
PERF_URLS ?= http://localhost:8080/ http://localhost:8080/organizer http://localhost:8080/organizer/agenda http://localhost:8080/organizer/portal http://localhost:8080/public/m31-systems-forum-2026/agenda

dev:
	DEMO_MODE=memory go run .

test:
	go list ./... | rg -v '/dist(/|$$)' | xargs go test
	go list ./... | rg -v '/dist(/|$$)' | xargs go test -race

check:
	test -z "$$(gofmt -l $$(rg --files -g '*.go' -g '!dist/**' -g '!build/**'))"
	$(GOSX) fmt --check app
	for policy_file in rules/*.arb; do arbiter check "$$policy_file"; done
	go list ./... | rg -v '/dist(/|$$)' | xargs go vet
	go list ./... | rg -v '/dist(/|$$)' | xargs go test
	go list ./... | rg -v '/dist(/|$$)' | xargs go test -race

build:
	rm -rf -- dist
	$(GOSX) build --prod .
	find dist/assets/runtime -type f -name '*.map' -delete
	find dist/app -type f -name '*.go' -delete

size-budget: build
	GOSX=$(GOSX) go run ./cmd/sizecheck -root .

perf-budget:
	$(GOSX) perf --mobile pixel7 --throttle 4 --coverage --budget perf-budget.json $(PERF_URLS)

release-check: check size-budget
