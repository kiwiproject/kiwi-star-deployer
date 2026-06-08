BINARY      := kiwi-star-deployer
INSTALL_DIR ?= $(shell go env GOPATH)/bin
VERSION     := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS     := -X github.com/kiwiproject/kiwi-star-deployer/cmd.version=$(VERSION)

.PHONY: build install test vet lint check fmt tidy clean help

help:
	@echo "Targets:"
	@echo "  build    build the binary"
	@echo "  install  build and install to $(INSTALL_DIR)"
	@echo "  test     run tests"
	@echo "  vet      run go vet"
	@echo "  lint     run golangci-lint"
	@echo "  check    vet + test + lint (matches CI)"
	@echo "  fmt      format all Go source files"
	@echo "  tidy     run go mod tidy"
	@echo "  clean    remove the built binary"
	@echo ""
	@echo "Override install location: make install INSTALL_DIR=/usr/local/bin"

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) .

install: build
	@test -d "$(INSTALL_DIR)" || { echo "Error: INSTALL_DIR '$(INSTALL_DIR)' does not exist"; exit 1; }
	cp $(BINARY) $(INSTALL_DIR)/$(BINARY)
	@echo "Installed $(BINARY) to $(INSTALL_DIR)"

test:
	go test -count=1 ./...

vet:
	go vet ./...

lint:
	golangci-lint run

check: vet test lint

fmt:
	gofmt -w ./...

tidy:
	go mod tidy

clean:
	rm -f $(BINARY)
