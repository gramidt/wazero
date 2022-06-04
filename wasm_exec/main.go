package wasm_exec

import (
	"context"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// WasmExec allows you to run wasm compiled with `GOARCH=wasm GOOS=js`.
type WasmExec interface {
	// Run instantiates a new module and calls the "run" export with the given module config.
	Run(context.Context, wazero.CompiledModule, wazero.ModuleConfig) error

	api.Closer
}

type wasmExec struct{ r wazero.Runtime }

func newWasmExec(r wazero.Runtime) WasmExec {
	return &wasmExec{r}
}

// Run implements WasmExec.Run
func (e *wasmExec) Run(ctx context.Context, compiled wazero.CompiledModule, mConfig wazero.ModuleConfig) error {
	// Instantiate the imports needed by go-compiled wasm.
	// TODO: We can't currently share a compiled module because the one here is stateful.
	js, err := moduleBuilder(e.r).Instantiate(ctx, e.r)
	if err != nil {
		return err
	}
	defer js.Close(ctx)

	// Instantiate the module compiled by go, noting it has no init function.
	mod, err := e.r.InstantiateModule(ctx, compiled, mConfig)
	if err != nil {
		return err
	}
	defer mod.Close(ctx)

	// Extract the args and env from the module config and write it to memory.
	s := &state{values: &values{ids: map[interface{}]uint32{}}}
	ctx = context.WithValue(ctx, stateKey{}, s)
	argc, argv, err := writeArgsAndEnviron(ctx, mod)
	if err != nil {
		return err
	}
	// Invoke the run function.
	_, err = mod.ExportedFunction("run").Call(ctx, uint64(argc), uint64(argv))
	return err
}

// Close implements WasmExec.Close
func (e *wasmExec) Close(context.Context) error {
	// currently no-op
	return nil
}
