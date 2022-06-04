package wasm_exec

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/wasm"
)

const (
	functionWasmExit             = "runtime.wasmExit"
	functionWasmWrite            = "runtime.wasmWrite"
	functionResetMemoryDataView  = "runtime.resetMemoryDataView"
	functionNanotime1            = "runtime.nanotime1"
	functionWalltime             = "runtime.walltime"
	functionScheduleTimeoutEvent = "runtime.scheduleTimeoutEvent"
	functionClearTimeoutEvent    = "runtime.clearTimeoutEvent"
	functionGetRandomData        = "runtime.getRandomData"
)

// wasmExit implements runtime.wasmExit which supports runtime.exit.
//
// See https://github.com/golang/go/blob/go1.19rc2/src/runtime/sys_wasm.go#L28
func wasmExit(ctx context.Context, mod api.Module, sp uint32) {
	code := mustReadUint32Le(ctx, mod.Memory(), "code", sp+8)
	getState(ctx).clear()
	_ = mod.CloseWithExitCode(ctx, code)
}

// wasmWrite implements runtime.wasmWrite which supports runtime.write and
// runtime.writeErr. It is only known to be used with fd = 2 (stderr).
//
// See https://github.com/golang/go/blob/go1.19rc2/src/runtime/os_js.go#L29
func wasmWrite(ctx context.Context, mod api.Module, sp uint32) {
	fd := mustReadUint64Le(ctx, mod.Memory(), "fd", sp+8)
	p := mustReadUint64Le(ctx, mod.Memory(), "p", sp+16)
	n := mustReadUint32Le(ctx, mod.Memory(), "n", sp+24)

	var writer io.Writer

	switch fd {
	case 1:
		writer = mod.(*wasm.CallContext).Sys.Stdout()
	case 2:
		writer = mod.(*wasm.CallContext).Sys.Stderr()
	default:
		// Keep things simple by expecting nothing past 2
		panic(fmt.Errorf("unexpected fd %d", fd))
	}

	if _, err := writer.Write(mustRead(ctx, mod.Memory(), "p", uint32(p), n)); err != nil {
		panic(fmt.Errorf("error writing p: %w", err))
	}
}

// resetMemoryDataView signals wasm.OpcodeMemoryGrow happened, indicating any
// cached view of memory should be reset.
//
// See https://github.com/golang/go/blob/9839668b5619f45e293dd40339bf0ac614ea6bee/src/runtime/mem_js.go#L82
var resetMemoryDataView = &wasm.Func{
	ExportNames: []string{functionResetMemoryDataView},
	Name:        functionResetMemoryDataView,
	ParamTypes:  []wasm.ValueType{wasm.ValueTypeI32},
	ParamNames:  []string{parameterSp},
	// TODO: Compiler-based memory.grow callbacks are ignored until we have a generic solution #601
	Code: &wasm.Code{Body: []byte{wasm.OpcodeEnd}},
}

// nanotime1 implements runtime.nanotime which supports time.Since.
//
// See https://github.com/golang/go/blob/go1.19rc2/src/runtime/sys_wasm.s#L184
func nanotime1(ctx context.Context, mod api.Module, sp uint32) {
	nanos := uint64(mod.(*wasm.CallContext).Sys.Nanotime(ctx))
	mustWriteUint64Le(ctx, mod.Memory(), "t", sp+8, nanos)
}

// walltime implements runtime.walltime which supports time.Now.
//
// See https://github.com/golang/go/blob/go1.19rc2/src/runtime/sys_wasm.s#L188
func walltime(ctx context.Context, mod api.Module, sp uint32) {
	sec, nsec := mod.(*wasm.CallContext).Sys.Walltime(ctx)
	mustWriteUint64Le(ctx, mod.Memory(), "sec", sp+8, uint64(sec))
	mustWriteUint64Le(ctx, mod.Memory(), "nsec", sp+16, uint64(nsec))
}

// scheduleTimeoutEvent implements runtime.scheduleTimeoutEvent which supports
// runtime.notetsleepg used by runtime.signal_recv.
//
// Unlike other most functions prefixed by "runtime.", this both launches a
// goroutine and invokes code compiled into wasm "resume".
//
// See https://github.com/golang/go/blob/go1.19rc2/src/runtime/sys_wasm.s#L192
func scheduleTimeoutEvent(ctx context.Context, mod api.Module, sp uint32) {
	delayMs := mustReadUint64Le(ctx, mod.Memory(), "delay", sp+8)
	delay := time.Duration(delayMs) * time.Millisecond

	resume := mod.ExportedFunction("resume")

	// Invoke resume as an anonymous function, to propagate the context.
	callResume := func() {
		// This may seem like it can occur on a different goroutine, but
		// wasmexec is not goroutine-safe, so it won't.
		if _, err := resume.Call(ctx); err != nil {
			panic(err)
		}
	}

	id := getState(ctx).scheduleEvent(delay, callResume)
	mustWriteUint64Le(ctx, mod.Memory(), "id", sp+16, uint64(id))
}

// clearTimeoutEvent implements runtime.clearTimeoutEvent which supports
// runtime.notetsleepg used by runtime.signal_recv.
//
// See https://github.com/golang/go/blob/go1.19rc2/src/runtime/sys_wasm.s#L196
func clearTimeoutEvent(ctx context.Context, mod api.Module, sp uint32) {
	id := mustReadUint32Le(ctx, mod.Memory(), "id", sp+8)
	getState(ctx).clearTimeoutEvent(id)
}

// getRandomData implements runtime.getRandomData, which initializes the seed
// for runtime.fastrand.
//
// See https://github.com/golang/go/blob/go1.19rc2/src/runtime/sys_wasm.s#L200
func getRandomData(ctx context.Context, mod api.Module, sp uint32) {
	buf := uint32(mustReadUint64Le(ctx, mod.Memory(), "buf", sp+8))
	bufLen := uint32(mustReadUint64Le(ctx, mod.Memory(), "bufLen", sp+16))

	randSource := mod.(*wasm.CallContext).Sys.RandSource()

	r := mustRead(ctx, mod.Memory(), "r", buf, bufLen)

	if n, err := randSource.Read(r); err != nil {
		panic(fmt.Errorf("RandSource.Read(r /* len =%d */) failed: %w", bufLen, err))
	} else if uint32(n) != bufLen {
		panic(fmt.Errorf("RandSource.Read(r /* len=%d */) read %d bytes", bufLen, n))
	}
}
