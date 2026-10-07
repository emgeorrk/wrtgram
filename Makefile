BIN_DIR   := bin
DIST_DIR  := dist
BINARY    := $(BIN_DIR)/wrtgram
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS   := -s -w -X main.version=$(VERSION)
GOFLAGS   := -trimpath
ROUTER    ?= 192.168.1.1
ROUTER_GOARCH ?= arm64
# Size budget for the arm64 binary, bytes (CI gate, see .github/workflows/ci.yml).
SIZE_MAX  := 9437184

# GOOS/GOARCH/extra-env triples covering the common OpenWrt targets.
DIST_TARGETS := \
	linux_arm64:arm64: \
	linux_armv7:arm:GOARM=7 \
	linux_armv5:arm:GOARM=5 \
	linux_mipsle:mipsle:GOMIPS=softfloat \
	linux_mips:mips:GOMIPS=softfloat \
	linux_amd64:amd64: \
	linux_386:386:GO386=softfloat \
	linux_riscv64:riscv64:

export CGO_ENABLED := 0

.PHONY: all build build-router dist size deploy install-remote rss run-fake generate test vet lint lint-fix clean help

all: build ### build for the host (default)

build: ### compile for the host OS (dev/tests)
	go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BINARY) ./cmd/wrtgram

build-router: ### compile a static Linux binary for the router ($(ROUTER_GOARCH))
	GOOS=linux GOARCH=$(ROUTER_GOARCH) go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BINARY)-linux-$(ROUTER_GOARCH) ./cmd/wrtgram

dist: ### cross-compile every target into $(DIST_DIR)/
	@mkdir -p $(DIST_DIR)
	@for t in $(DIST_TARGETS); do \
		name=$${t%%:*}; rest=$${t#*:}; arch=$${rest%%:*}; extra=$${rest#*:}; \
		echo "  $$name"; \
		env GOOS=linux GOARCH=$$arch $$extra go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(DIST_DIR)/wrtgram-$$name ./cmd/wrtgram || exit 1; \
	done
	@ls -l $(DIST_DIR)

size: build-router ### print the router binary size and check it against the budget
	@sz=$$(stat -f %z $(BINARY)-linux-$(ROUTER_GOARCH) 2>/dev/null || stat -c %s $(BINARY)-linux-$(ROUTER_GOARCH)); \
	echo "$(BINARY)-linux-$(ROUTER_GOARCH): $$sz bytes ($$((sz / 1024 / 1024)) MiB)"; \
	[ "$$sz" -le $(SIZE_MAX) ] || { echo "binary exceeds the size budget ($(SIZE_MAX))"; exit 1; }

deploy: build-router ### copy the router binary to $(ROUTER):/tmp/wrtgram and run probe + check-config
	scp -O $(BINARY)-linux-$(ROUTER_GOARCH) root@$(ROUTER):/tmp/wrtgram
	ssh root@$(ROUTER) '/tmp/wrtgram probe && /tmp/wrtgram check-config'

install-remote: deploy ### install the deployed binary into /usr/bin and restart the service
	ssh root@$(ROUTER) 'cp /tmp/wrtgram /usr/bin/wrtgram && chmod 755 /usr/bin/wrtgram && /etc/init.d/wrtgram restart'

rss: ### print the resident memory of the running bot on $(ROUTER)
	ssh root@$(ROUTER) 'for p in $$(pidof wrtgram); do grep -E "VmRSS|VmHWM" /proc/$$p/status; done'

run-fake: ### run on the host against recorded fixtures (needs WRTGRAM_TOKEN and WRTGRAM_CHAT_IDS)
	WRTGRAM_FAKE=1 go run ./cmd/wrtgram run

generate: ### regenerate gomock mocks (go generate ./...)
	go generate ./...

test: ### unit tests
	go test -race ./...

vet: ### static analysis
	go vet ./...

lint: ### golangci-lint
	golangci-lint run

lint-fix: ### golangci-lint with autofix
	golangci-lint run --fix

clean: ### remove build artifacts
	rm -rf $(BIN_DIR) $(DIST_DIR)

help: ### list targets
	@grep -E '^[a-zA-Z_-]+:.*### .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*### "}; {printf "  %-15s %s\n", $$1, $$2}'
