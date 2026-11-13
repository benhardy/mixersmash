//go:build ignore

// web_server.go is a standalone local HTTP server for testing the WASM build.
// Run it with: go run web_server.go
// Or via:      make serve
//
// It serves the web/ directory (or the directory given by -dir) and ensures
// .wasm files are delivered with the correct Content-Type so that
// WebAssembly.instantiateStreaming works in all browsers.

package main

import (
	"flag"
	"log"
	"mime"
	"net/http"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "address to listen on")
	dir := flag.String("dir", "web", "directory to serve")
	flag.Parse()

	// Guarantee the correct MIME type for .wasm regardless of the host OS
	// mime table. A wrong type causes WebAssembly.instantiateStreaming to
	// reject the response.
	if err := mime.AddExtensionType(".wasm", "application/wasm"); err != nil {
		log.Fatalf("mime.AddExtensionType: %v", err)
	}

	fs := http.FileServer(http.Dir(*dir))
	http.Handle("/", fs)

	log.Printf("serving %s at http://%s", *dir, *addr)
	if err := http.ListenAndServe(*addr, nil); err != nil {
		log.Fatalf("ListenAndServe: %v", err)
	}
}
