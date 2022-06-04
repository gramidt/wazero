package main

import (
	"context"
	"io/fs"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/testing/maintester"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/sys"
	"github.com/tetratelabs/wazero/wasm_exec"
)

// Test_main ensures the following will work:
//
//	go run cat.go /test.txt
func Test_main(t *testing.T) {
	stdout, stderr := maintester.TestMain(t, main, "cat", "/test.txt")
	require.Equal(t, "", stderr)
	require.Equal(t, "greet filesystem\n", stdout)
}

func Benchmark_main(b *testing.B) {
	// Choose the context to use for function calls.
	ctx := context.Background()

	// Create a new WebAssembly Runtime.
	r := wazero.NewRuntimeWithConfig(wazero.NewRuntimeConfigCompiler())
	defer r.Close(ctx) // This closes everything this Runtime created.

	// Compile the WebAssembly module using the default configuration.
	compiled, err := r.CompileModule(ctx, catWasm, wazero.NewCompileConfig())
	if err != nil {
		b.Fatal(err)
	}

	we, err := wasm_exec.NewBuilder().Build(ctx, r)

	rooted, err := fs.Sub(catFS, "testdata")
	if err != nil {
		b.Fatal(err)
	}
	config := wazero.NewModuleConfig().WithFS(rooted).WithArgs("cat", "/test.txt")

	b.Run("go cat", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			err = we.Run(ctx, compiled, config)
			if exitErr, ok := err.(*sys.ExitError); ok && exitErr.ExitCode() != 0 {
				b.Fatal(err)
			} else if !ok {
				b.Fatal(err)
			}
		}
	})
}
