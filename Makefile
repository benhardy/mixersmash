.PHONY: all mac linux wasm serve test lint clean hooks analyse

mac: mixersmash_darwin_arm64

mixersmash_darwin_arm64:
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o mixersmash_darwin_arm64 .

linux: mixersmash_linux_amd64

mixersmash_linux_amd64:
	GOOS=linux GOARCH=amd64 go build -o mixersmash_linux_amd64 .

# wasm builds the WebAssembly binary and copies the Go JS shim into web/.
# Go 1.24+ ships wasm_exec.js under lib/wasm/; older Go uses misc/wasm/.
wasm:
	GOOS=js GOARCH=wasm CGO_ENABLED=0 go build -o web/mixersmash.wasm .
	@if [ -f "$(shell go env GOROOT)/lib/wasm/wasm_exec.js" ]; then \
		cp "$(shell go env GOROOT)/lib/wasm/wasm_exec.js" web/wasm_exec.js; \
	else \
		cp "$(shell go env GOROOT)/misc/wasm/wasm_exec.js" web/wasm_exec.js; \
	fi
	echo "window.MIXERSMASH_WASM_SIZE = $$(wc -c < web/mixersmash.wasm | tr -d ' \t\n');" > web/wasm-meta.js

# serve starts a local HTTP server for the web/ directory.
# A static file server is required: browsers cannot fetch .wasm via file://.
# Run `make wasm` first to build the WebAssembly binary before serving.
serve:
	go run tools/web_server/main.go

test:
	go test ./...

lint:
	golangci-lint run ./...

hooks:
	git config core.hooksPath .githooks
	@echo "pre-commit hook activated (core.hooksPath = .githooks)"

clean:
	rm -f mixersmash_darwin_arm64 mixersmash_linux_amd64
	rm -f web/mixersmash.wasm web/wasm_exec.js web/wasm-meta.js

# analyse runs code-complexity analysis (cyclomatic, cognitive, maintainability).
# Non-zero exits from individual tools (threshold findings) are tolerated.
analyse:
	@echo "=== gocyclo (cyclomatic complexity, over 10) ==="
	-go run github.com/fzipp/gocyclo/cmd/gocyclo@latest -over 10 -avg .
	@echo "=== gocognit (cognitive complexity, over 10, top 15) ==="
	-go run github.com/uudashr/gocognit/cmd/gocognit@latest -over 10 -top 15 .
	@echo "=== maintidx (maintainability index; fetched via go run on first use) ==="
	-go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run --no-config --default=none --enable=maintidx ./...
