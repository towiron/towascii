BIN     := bin/towascii
PREFIX  ?= $(HOME)/.local
DEST    := $(PREFIX)/bin/towascii

.PHONY: build run test lint install uninstall clean

build:
	CGO_ENABLED=1 go build -o $(BIN) .

run: build
	./$(BIN)

test:
	go test ./...

lint:
	go vet ./...
	@if command -v golangci-lint >/dev/null; then golangci-lint run ./...; else echo "golangci-lint not installed, skipped"; fi

install: build
	mkdir -p $(dir $(DEST))
	cp $(BIN) $(DEST)
	$(DEST) install

uninstall:
	@if [ -x $(DEST) ]; then $(DEST) uninstall; fi
	rm -f $(DEST)

clean:
	rm -rf bin/
