.PHONY: all linux wasm serve test clean

all: go2d_darwin_arm64

go2d_darwin_arm64:
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -o go2d_darwin_arm64 .

linux: go2d_linux_amd64

go2d_linux_amd64:
	GOOS=linux GOARCH=amd64 go build -o go2d_linux_amd64 .

# wasm builds the WebAssembly binary and copies the Go JS shim into web/.
# Go 1.24+ ships wasm_exec.js under lib/wasm/; older Go uses misc/wasm/.
wasm:
	GOOS=js GOARCH=wasm CGO_ENABLED=0 go build -o web/go2d.wasm .
	@if [ -f "$(shell go env GOROOT)/lib/wasm/wasm_exec.js" ]; then \
		cp "$(shell go env GOROOT)/lib/wasm/wasm_exec.js" web/wasm_exec.js; \
	else \
		cp "$(shell go env GOROOT)/misc/wasm/wasm_exec.js" web/wasm_exec.js; \
	fi

# serve starts a local HTTP server for the web/ directory.
# A static file server is required: browsers cannot fetch .wasm via file://.
# Run `make wasm` first to build the WebAssembly binary before serving.
serve:
	go run web_server.go

test:
	go test ./...

clean:
	rm -f go2d_darwin_arm64 go2d_linux_amd64
	rm -f web/go2d.wasm web/wasm_exec.js
