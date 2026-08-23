# Version is derived from the git tag — `git describe` gives v0.1.0 at the
# tag and v0.1.0-3-gabc1234 three commits later. There is no version
# constant to hand-edit; `git tag vX.Y.Z` is the single source of truth.
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

.PHONY: install build test version

# install depends on build: the five status-*.sh hooks are registered
# straight from this repo checkout (see README's "Wiring it up"), never
# copied anywhere by this target, and each one self-locates render-status
# relative to its OWN script path -- i.e. this checkout, not $GOBIN. `make
# install` alone used to leave that repo-local ./render-status missing on
# a fresh clone (`make build` was a separate, undocumented-as-required
# step), so every hook failed with "No such file or directory", silently,
# behind the `2>/dev/null || true` every registration uses -- caught by an
# adversarial pressure-test panel reproducing it end to end. Depending on
# build here means `make install` alone always leaves a fresh binary at
# the path the hooks actually resolve.
install: build
	go install -ldflags "$(LDFLAGS)" ./cmd/render-status
	go install -ldflags "$(LDFLAGS)" ./cmd/lint-content

# Build local ./render-status and ./lint-content binaries with the version
# baked in.
build:
	go build -ldflags "$(LDFLAGS)" -o render-status ./cmd/render-status
	go build -ldflags "$(LDFLAGS)" -o lint-content ./cmd/lint-content

test:
	go test ./...

# Print the version that a build would stamp.
version:
	@echo $(VERSION)
