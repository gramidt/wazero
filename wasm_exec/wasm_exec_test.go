package wasm_exec_test

import (
	"context"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/sys"
)

// testCtx is an arbitrary, non-default context. Non-nil also prevents linter errors.
var testCtx = context.WithValue(context.Background(), struct{}{}, "arbitrary")

func Test_argsAndEnv(t *testing.T) {
	stdout, stderr, err := compileAndRunJsWasm(testCtx, `package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println()
	for i, a := range os.Args {
		fmt.Println("args", i, "=", a)
	}
	for i, e := range os.Environ() {
		fmt.Println("environ", i, "=", e)
	}
}`, wazero.NewModuleConfig().WithArgs("a", "b").WithEnv("c", "d").WithEnv("a", "b"))

	require.Error(t, err)
	require.Zero(t, err.(*sys.ExitError).ExitCode())
	require.Equal(t, `
args 0 = a
args 1 = b
environ 0 = c=d
environ 1 = a=b
`, stdout)
	require.Equal(t, "", stderr)
}
