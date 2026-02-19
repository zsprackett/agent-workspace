.PHONY: build test clean install

BINARY := agent-workspace
INSTALL_DIR := $(HOME)/.local/bin

build:
	go build -o $(BINARY) .

test:
	go test ./...

clean:
	rm -f $(BINARY)

install: build
	install -d $(INSTALL_DIR)
	install -m 755 $(BINARY) $(INSTALL_DIR)/$(BINARY)
