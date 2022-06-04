package main

import (
	"context"
	"embed"
	_ "embed"
	"io/fs"
	"log"
	"os"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/sys"
	"github.com/tetratelabs/wazero/wasm_exec"
)

// catFS is an embedded filesystem limited to test.txt
//go:embed testdata/test.txt
var catFS embed.FS

// catWasm was compiled the TinyGo source testdata/cat.go
//go:embed testdata/cat.wasm
var catWasm []byte

// main writes an input file to stdout, just like `cat`.
//
// This is a basic introduction to the WebAssembly System Interface (WASI).
// See https://github.com/WebAssembly/WASI
func main() {
	// Choose the context to use for function calls.
	ctx := context.Background()

	// Create a new WebAssembly Runtime.
	r := wazero.NewRuntime()
	defer r.Close(ctx) // This closes everything this Runtime created.

	// Compile the WebAssembly module using the default configuration.
	compiled, err := r.CompileModule(ctx, catWasm, wazero.NewCompileConfig())
	if err != nil {
		log.Panicln(err)
	}

	we, err := wasm_exec.NewBuilder().Build(ctx, r)
	if err != nil {
		log.Panicln(err)
	}

	// Since wazero uses fs.FS, we can use standard libraries to do things like trim the leading path.
	rooted, err := fs.Sub(catFS, "testdata")
	if err != nil {
		log.Panicln(err)
	}

	// Create a configuration for running main, overriding defaults (which discard stdout and has no file system).
	config := wazero.NewModuleConfig().
		WithFS(rooted).
		WithStdout(os.Stdout).
		WithStderr(os.Stderr).
		WithArgs("cat", os.Args[1])

	err = we.Run(ctx, compiled, config)
	if exitErr, ok := err.(*sys.ExitError); ok && exitErr.ExitCode() != 0 {
		log.Panicln(err)
	} else if !ok {
		log.Panicln(err)
	}
}
