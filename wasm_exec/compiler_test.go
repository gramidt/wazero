package wasm_exec_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/binary"
	"github.com/tetratelabs/wazero/sys"
	"github.com/tetratelabs/wazero/wasm_exec"
)

// Test_compileJsWasm ensures the infrastructure to generate wasm on-demand works.
func Test_compileJsWasm(t *testing.T) {
	bin, err := compileJsWasm(`package main

import "os"

func main() {
	os.Exit(1)
}`)
	require.NoError(t, err)

	m, err := binary.DecodeModule(bin, wasm.Features20191205, wasm.MemorySizer)
	require.NoError(t, err)
	// TODO: implement go.buildid custom section and validate it instead.
	require.NotNil(t, m.MemorySection)
}

func Test_compileAndRunJsWasm(t *testing.T) {
	stdout, stderr, err := compileAndRunJsWasm(testCtx, `package main

import "os"

func main() {
	os.Stdout.Write([]byte("stdout"))
	os.Stderr.Write([]byte("stderr"))
	os.Exit(1)
}`, wazero.NewModuleConfig())

	require.Equal(t, "stdout", stdout)
	require.Equal(t, "stderr", stderr)
	require.Error(t, err)
	require.Equal(t, uint32(1), err.(*sys.ExitError).ExitCode())
}

func compileAndRunJsWasm(ctx context.Context, goSrc string, config wazero.ModuleConfig) (stdout, stderr string, err error) {
	bin, compileJsErr := compileJsWasm(goSrc)
	if compileJsErr != nil {
		err = compileJsErr
		return
	}

	r := wazero.NewRuntime()
	defer r.Close(ctx)

	compiled, compileErr := r.CompileModule(ctx, bin, wazero.NewCompileConfig())
	if compileErr != nil {
		err = compileErr
		return
	}

	we, newErr := wasm_exec.NewBuilder().Build(ctx, r)
	if newErr != nil {
		err = newErr
		return
	}

	stdoutBuf, stderrBuf := &bytes.Buffer{}, &bytes.Buffer{}
	err = we.Run(ctx, compiled, config.WithStdout(stdoutBuf).WithStderr(stderrBuf))
	stdout = stdoutBuf.String()
	stderr = stderrBuf.String()
	return
}

// compileJsWasm allows us to generate a binary with runtime.GOOS=js and runtime.GOARCH=wasm.
func compileJsWasm(goSrc string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	goBin, err := findGoBin()
	if err != nil {
		return nil, err
	}

	workDir, err := os.MkdirTemp("", "")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(workDir)

	bin := "out.wasm"
	goArgs := []string{"build", "-o", bin, "."}
	if err = os.WriteFile(filepath.Join(workDir, "main.go"), []byte(goSrc), 0o600); err != nil {
		return nil, err
	}

	if err = os.WriteFile(filepath.Join(workDir, "go.mod"),
		[]byte("module github.com/tetratelabs/wazero/wasm_exec/examples\n\ngo 1.17\n"), 0o600); err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, goBin, goArgs...) //nolint:gosec
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("couldn't compile %s: %s\n%w", bin, string(out), err)
	}

	binBytes, err := os.ReadFile(filepath.Join(workDir, bin)) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("couldn't compile %s: %w", bin, err)
	}
	return binBytes, nil
}

func findGoBin() (string, error) {
	binName := "go"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	goBin := filepath.Join(runtime.GOROOT(), "bin", binName)
	if _, err := os.Stat(goBin); err == nil {
		return goBin, nil
	}
	// Now, search the path
	return exec.LookPath(binName)
}
