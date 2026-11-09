BIN        := $(CURDIR)/bin
PRESET     ?= debug
ENGINE_BIN := engine/build/$(PRESET)/src/strix_engine

GO_SOURCES     := $(shell find controlplane -name '*.go' -o -name go.mod -o -name go.sum)
ENGINE_SOURCES := $(shell find engine/src engine/include proto -type f 2>/dev/null) engine/CMakeLists.txt

# Changes whenever any control-plane source changes, so a daemon that outlives a rebuild
# is detected by the client (X-Strix-Build mismatch), not only after a new commit.
BUILD_ID := $(shell git rev-parse --short HEAD 2>/dev/null || echo nogit)-$(shell cat $(GO_SOURCES) 2>/dev/null | sha256sum | cut -c1-8)

LDFLAGS := -X strix/internal/wire/control.BuildID=$(BUILD_ID)

.PHONY: strix build go engine clean

# Recommended entry point: build incrementally, then replace make's shell with the daemon
# (foreground, logs on this terminal). Ctrl-C / SIGTERM must reach the daemon's quit path.
strix: build
	exec $(BIN)/strix daemon

build: go engine

go: $(BIN)/strix $(BIN)/strix_gateway

$(BIN)/strix: $(GO_SOURCES)
	cd controlplane && go build -ldflags '$(LDFLAGS)' -o $@ ./cmd/supervisor

$(BIN)/strix_gateway: $(GO_SOURCES)
	cd controlplane && go build -o $@ ./cmd/gateway

# CMake/Ninja own engine incrementality; the copy is skipped when nothing changed.
engine: $(BIN)/strix_engine

$(BIN)/strix_engine: $(ENGINE_SOURCES)
	cd engine && cmake --preset $(PRESET) && cmake --build --preset $(PRESET) --target strix_engine
	mkdir -p $(BIN)
	cp -f $(ENGINE_BIN) $@

clean:
	rm -rf $(BIN)
