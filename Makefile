APP := cathode

# Install location. Defaults to ~/.local/bin since that's already on PATH on
# most macOS / Linux setups and doesn't need sudo. Override with PREFIX:
#   make install PREFIX=/usr/local/bin
PREFIX ?= $(HOME)/.local/bin

.PHONY: build run test tidy clean install uninstall reinstall watch

build: ## compile the single binary
	go build -o $(APP) .

run: build ## build then launch (ask mode)
	./$(APP) -mode ask

test: ## run the unit tests
	go test ./...

tidy: ## resolve deps + write go.sum
	go mod tidy

install: ## build and copy to $(PREFIX), creating it if needed
	@mkdir -p $(PREFIX)
	go build -o $(PREFIX)/$(APP) .
	@echo "installed: $(PREFIX)/$(APP)"
	@case ":$$PATH:" in *":$(PREFIX):"*) ;; *) \
	  echo "warn: $(PREFIX) is not on PATH — add it to ~/.zshrc or ~/.bashrc";; esac

reinstall: clean install ## rebuild from scratch and reinstall

uninstall: ## remove the installed binary
	@rm -f $(PREFIX)/$(APP) && echo "removed: $(PREFIX)/$(APP)" || true

watch: ## rebuild + reinstall on every *.go save (needs entr: `brew install entr`)
	@command -v entr >/dev/null || { echo "entr not found — run: brew install entr"; exit 1; }
	@ls *.go | entr -rc $(MAKE) install

clean:
	rm -f $(APP)
