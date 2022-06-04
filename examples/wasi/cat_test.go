package main

import (
	"context"
	"io/fs"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/testing/maintester"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/wasi_snapshot_preview1"
)

// Test_main ensures the following will work:
//
//	go run cat.go /test.txt
func Test_main(t *testing.T) {
	for _, toolchain := range []string{"cargo-wasi", "tinygo", "zig-cc"} {
		toolchain := toolchain
		t.Run(toolchain, func(t *testing.T) {
			t.Setenv("TOOLCHAIN", toolchain)
			stdout, stderr := maintester.TestMain(t, main, "cat", "/test.txt")
			require.Equal(t, "", stderr)
			require.Equal(t, "greet filesystem\n", stdout)
		})
	}
}

func Benchmark_main(b *testing.B) {
	// Choose the context to use for function calls.
	ctx := context.Background()

	// Create a new WebAssembly Runtime.
	r := wazero.NewRuntimeWithConfig(wazero.NewRuntimeConfig().
		// Enable WebAssembly 2.0 support, which is required for TinyGo 0.24+.
		WithWasmCore2())
	defer r.Close(ctx) // This closes everything this Runtime created.

	// Instantiate WASI, which implements system I/O such as console output.
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, r); err != nil {
		b.Fatal(err)
	}

	// Compile the WebAssembly module using the default configuration.
	code, err := r.CompileModule(ctx, catWasmTinyGo, wazero.NewCompileConfig())
	if err != nil {
		b.Fatal(err)
	}

	rooted, err := fs.Sub(catFS, "testdata")
	if err != nil {
		b.Fatal(err)
	}
	config := wazero.NewModuleConfig().WithFS(rooted).WithArgs("cat", "/test.txt")

	b.Run("tinygo cat", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if mod, err := r.InstantiateModule(ctx, code, config); err != nil {
				b.Fatal(err)
			} else {
				mod.Close(ctx)
			}
		}
	})
}
