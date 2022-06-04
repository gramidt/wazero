package wasm_exec_test

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/sys"
	"github.com/tetratelabs/wazero/wasm_exec"
)

// This is an example of how to use `GOARCH=wasm GOOS=js` compiled wasm via a
// sleep function.
//
// See https://github.com/tetratelabs/wazero/tree/main/examples/wasm_exec for another example.
func Example() {
	// Choose the context to use for function calls.
	ctx := context.Background()

	// Create a new WebAssembly Runtime.
	r := wazero.NewRuntime()
	defer r.Close(ctx)

	// Compile the source as GOARCH=wasm GOOS=js.
	bin, err := compileJsWasm(`package main

import "time"

func main() {
	time.Sleep(time.Duration(1))
}`)
	if err != nil {
		log.Panicln(err)
	}

	compiled, err := r.CompileModule(ctx, bin, wazero.NewCompileConfig())
	if err != nil {
		log.Panicln(err)
	}

	// Create a wasm_exec runner.
	we, err := wasm_exec.NewBuilder().Build(ctx, r)
	if err != nil {
		log.Panicln(err)
	}

	// Override defaults which discard stdout and fake sleep.
	config := wazero.NewModuleConfig().
		WithStdout(os.Stdout).
		WithStderr(os.Stderr)

	err = we.Run(ctx, compiled, config)
	if exitErr, ok := err.(*sys.ExitError); ok {
		// Print the exit code
		fmt.Printf("exit_code: %d\n", exitErr.ExitCode())
	} else if !ok {
		log.Panicln(err)
	}
	// Output:
	// exit_code: 0
}
