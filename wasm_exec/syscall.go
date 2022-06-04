package wasm_exec

import (
	"context"
	"fmt"
	"io"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/wasm"
)

const (
	functionFinalizeRef        = "syscall/js.finalizeRef"
	functionStringVal          = "syscall/js.stringVal"
	functionValueGet           = "syscall/js.valueGet"
	functionValueSet           = "syscall/js.valueSet"
	functionValueDelete        = "syscall/js.valueDelete"
	functionValueIndex         = "syscall/js.valueIndex"
	functionValueSetIndex      = "syscall/js.valueSetIndex"
	functionValueCall          = "syscall/js.valueCall"
	functionValueInvoke        = "syscall/js.valueInvoke"
	functionValueNew           = "syscall/js.valueNew"
	functionValueLength        = "syscall/js.valueLength"
	functionValuePrepareString = "syscall/js.valuePrepareString"
	functionValueLoadString    = "syscall/js.valueLoadString"
	functionValueInstanceOf    = "syscall/js.valueInstanceOf"
	functionCopyBytesToGo      = "syscall/js.copyBytesToGo"
	functionCopyBytesToJS      = "syscall/js.copyBytesToJS"
)

// finalizeRef implements js.finalizeRef, which is used as a
// runtime.SetFinalizer on the given reference.
//
// See https://github.com/golang/go/blob/go1.19rc2/src/syscall/js/js.go#L61
func finalizeRef(ctx context.Context, mod api.Module, sp uint32) {
	// 32-bits are the ID
	id := mustReadUint32Le(ctx, mod.Memory(), "r", sp+8)
	getState(ctx).values.decrement(id)
	panic("TODO: generate code that uses this or stub it as unused")
}

// stringVal implements js.stringVal, which is used to load the string for
// `js.ValueOf(x)`.
//
// See https://github.com/golang/go/blob/go1.19rc2/src/syscall/js/js.go#L212
//     https://github.com/golang/go/blob/go1.19rc2/misc/wasm/wasm_exec.js#L305-L308
var stringVal = wasm.NewGoFunc(
	functionStringVal, functionStringVal,
	[]string{parameterSp},
	func(ctx context.Context, mod api.Module, sp uint32) {
		xAddr := mustReadUint64Le(ctx, mod.Memory(), "xAddr", sp+8)
		xLen := mustReadUint64Le(ctx, mod.Memory(), "xLen", sp+16)
		x := string(mustRead(ctx, mod.Memory(), "x", uint32(xAddr), uint32(xLen)))
		xRef := storeRef(ctx, mod, x)
		mustWriteUint64Le(ctx, mod.Memory(), "xRef", sp+24, xRef)
		// TODO: this is only used in the cat example: make a unit test
	},
)

// valueGet implements js.valueGet, which is used to load a js.Value property
// by name, ex. `v.Get("address")`.
//
// See https://github.com/golang/go/blob/go1.19rc2/src/syscall/js/js.go#L295
//     https://github.com/golang/go/blob/go1.19rc2/misc/wasm/wasm_exec.js#L311-L316
var valueGet = wasm.NewGoFunc(
	functionValueGet, functionValueGet,
	[]string{parameterSp},
	func(ctx context.Context, mod api.Module, sp uint32) {
		vRef := mustReadUint64Le(ctx, mod.Memory(), "vRef", sp+8)
		pAddr := mustReadUint64Le(ctx, mod.Memory(), "pAddr", sp+16)
		pLen := mustReadUint64Le(ctx, mod.Memory(), "pLen", sp+24)
		v := loadValue(ctx, mod, ref(vRef))
		p := mustRead(ctx, mod.Memory(), "p", uint32(pAddr), uint32(pLen))
		result := reflectGet(ctx, v, string(p))
		xRef := storeRef(ctx, mod, result)
		sp = refreshSP(mod)
		mustWriteUint64Le(ctx, mod.Memory(), "xRef", sp+32, xRef)
	},
)

// valueSet implements js.valueSet, which is used to store a js.Value property
// by name, ex. `v.Set("address", a)`.
//
// See https://github.com/golang/go/blob/go1.19rc2/src/syscall/js/js.go#L309
//     https://github.com/golang/go/blob/go1.19rc2/misc/wasm/wasm_exec.js#L318-L322
func valueSet(ctx context.Context, mod api.Module, sp uint32) {
	vRef := mustReadUint64Le(ctx, mod.Memory(), "vRef", sp+8)
	pAddr := mustReadUint64Le(ctx, mod.Memory(), "pAddr", sp+16)
	pLen := mustReadUint64Le(ctx, mod.Memory(), "pLen", sp+24)
	xRef := mustReadUint64Le(ctx, mod.Memory(), "xRef", sp+32)
	v := loadValue(ctx, mod, ref(vRef))
	p := mustRead(ctx, mod.Memory(), "p", uint32(pAddr), uint32(pLen))
	x := loadValue(ctx, mod, ref(xRef))
	reflectSet(ctx, v, string(p), x)
}

// valueDelete implements js.valueDelete, which is used to delete a js.Value property
// by name, ex. `v.Delete("address")`.
//
// See https://github.com/golang/go/blob/go1.19rc2/src/syscall/js/js.go#L321
//     https://github.com/golang/go/blob/go1.19rc2/misc/wasm/wasm_exec.js#L325-L328
func valueDelete(ctx context.Context, mod api.Module, sp uint32) {
	vRef := mustReadUint64Le(ctx, mod.Memory(), "vRef", sp+8)
	pAddr := mustReadUint64Le(ctx, mod.Memory(), "pAddr", sp+16)
	pLen := mustReadUint64Le(ctx, mod.Memory(), "pLen", sp+24)
	v := loadValue(ctx, mod, ref(vRef))
	p := mustRead(ctx, mod.Memory(), "p", uint32(pAddr), uint32(pLen))
	reflectDeleteProperty(v, string(p))
}

// valueIndex implements js.valueIndex, which is used to load a js.Value property
// by name, ex. `v.Index(0)`.
//
// See https://github.com/golang/go/blob/go1.19rc2/src/syscall/js/js.go#L334
//     https://github.com/golang/go/blob/go1.19rc2/misc/wasm/wasm_exec.js#L331-L334
func valueIndex(ctx context.Context, mod api.Module, sp uint32) {
	vRef := mustReadUint64Le(ctx, mod.Memory(), "vRef", sp+8)
	i := mustReadUint64Le(ctx, mod.Memory(), "i", sp+16)
	v := loadValue(ctx, mod, ref(vRef))
	result := reflectGetIndex(v, uint32(i))
	xRef := storeRef(ctx, mod, result)
	sp = refreshSP(mod)
	mustWriteUint64Le(ctx, mod.Memory(), "xRef", sp+24, xRef)
}

// valueSetIndex implements js.valueSetIndex, which is used to store a js.Value property
// by name, ex. `v.SetIndex(0, a)`.
//
// See https://github.com/golang/go/blob/go1.19rc2/src/syscall/js/js.go#L348
//     https://github.com/golang/go/blob/go1.19rc2/misc/wasm/wasm_exec.js#L337-L340
func valueSetIndex(ctx context.Context, mod api.Module, sp uint32) {
	vRef := mustReadUint64Le(ctx, mod.Memory(), "vRef", sp+8)
	i := mustReadUint64Le(ctx, mod.Memory(), "i", sp+16)
	xRef := mustReadUint64Le(ctx, mod.Memory(), "xRef", sp+24)
	v := loadValue(ctx, mod, ref(vRef))
	x := loadValue(ctx, mod, ref(xRef))
	reflectSetIndex(v, uint32(i), x)
}

// valueCall implements js.valueCall, which is used to call a js.Value function
// by name, ex. `document.Call("createElement", "div")`.
//
// See https://github.com/golang/go/blob/go1.19rc2/src/syscall/js/js.go#L394
//     https://github.com/golang/go/blob/go1.19rc2/misc/wasm/wasm_exec.js#L343-L358
func valueCall(ctx context.Context, mod api.Module, sp uint32) {
	vRef := mustReadUint64Le(ctx, mod.Memory(), "vRef", sp+8)
	mAddr := mustReadUint64Le(ctx, mod.Memory(), "mAddr", sp+16)
	mLen := mustReadUint64Le(ctx, mod.Memory(), "mLen", sp+24)
	argsArray := mustReadUint64Le(ctx, mod.Memory(), "argsArray", sp+32)
	argsLen := mustReadUint64Le(ctx, mod.Memory(), "argsLen", sp+40)

	v := loadValue(ctx, mod, ref(vRef))
	propertyKey := string(mustRead(ctx, mod.Memory(), "property", uint32(mAddr), uint32(mLen)))
	args := loadSliceOfValues(ctx, mod, uint32(argsArray), uint32(argsLen))

	var xRef, ok uint64
	if result, err := reflectApply(ctx, mod, v, propertyKey, args); err != nil {
		xRef = storeRef(ctx, mod, err)
		ok = 0
	} else {
		xRef = storeRef(ctx, mod, result)
		ok = 1
	}

	sp = refreshSP(mod)
	mustWriteUint64Le(ctx, mod.Memory(), "xRef", sp+56, xRef)
	mustWriteUint64Le(ctx, mod.Memory(), "ok", sp+64, ok)
}

// valueInvoke implements js.valueInvoke, which is used to call a js.Value, ex.
// `add.Invoke(1, 2)`.
//
// See https://github.com/golang/go/blob/go1.19rc2/src/syscall/js/js.go#L413
//     https://github.com/golang/go/blob/go1.19rc2/misc/wasm/wasm_exec.js#L361-L375
func valueInvoke(ctx context.Context, mod api.Module, sp uint32) {
	vRef := mustReadUint64Le(ctx, mod.Memory(), "vRef", sp+8)
	argsArray := mustReadUint64Le(ctx, mod.Memory(), "argsArray", sp+16)
	argsLen := mustReadUint64Le(ctx, mod.Memory(), "argsLen", sp+24)

	v := loadValue(ctx, mod, ref(vRef))
	args := loadSliceOfValues(ctx, mod, uint32(argsArray), uint32(argsLen))

	var xRef, ok uint64
	if result, err := reflectApply(ctx, mod, v, nil, args); err != nil {
		xRef = storeRef(ctx, mod, err)
		ok = 0
	} else {
		xRef = storeRef(ctx, mod, result)
		ok = 1
	}

	sp = refreshSP(mod)
	mustWriteUint64Le(ctx, mod.Memory(), "xRef", sp+40, xRef)
	mustWriteUint64Le(ctx, mod.Memory(), "ok", sp+48, ok)
}

// valueNew implements js.valueNew, which is used to call a js.Value, ex.
// `array.New(2)`.
//
// See https://github.com/golang/go/blob/go1.19rc2/src/syscall/js/js.go#L432
//     https://github.com/golang/go/blob/go1.19rc2/misc/wasm/wasm_exec.js#L380-L391
func valueNew(ctx context.Context, mod api.Module, sp uint32) {
	vRef := mustReadUint64Le(ctx, mod.Memory(), "vRef", sp+8)
	argsArray := mustReadUint64Le(ctx, mod.Memory(), "argsArray", sp+16)
	argsLen := mustReadUint64Le(ctx, mod.Memory(), "argsLen", sp+24)

	v := loadValue(ctx, mod, ref(vRef))
	args := loadSliceOfValues(ctx, mod, uint32(argsArray), uint32(argsLen))
	var xRef, ok uint64
	if result, err := reflectConstruct(v, args); err != nil {
		xRef = storeRef(ctx, mod, err)
		ok = 0
	} else {
		xRef = storeRef(ctx, mod, result)
		ok = 1
	}

	sp = refreshSP(mod)
	mustWriteUint64Le(ctx, mod.Memory(), "xRef", sp+40, xRef)
	mustWriteUint64Le(ctx, mod.Memory(), "ok", sp+48, ok)
}

// valueLength implements js.valueLength, which is used to load the length
// property of a value, ex. `array.length`.
//
// See https://github.com/golang/go/blob/go1.19rc2/src/syscall/js/js.go#L372
//     https://github.com/golang/go/blob/go1.19rc2/misc/wasm/wasm_exec.js#L396-L397
func valueLength(ctx context.Context, mod api.Module, sp uint32) {
	vRef := mustReadUint64Le(ctx, mod.Memory(), "vRef", sp+8)
	v := loadValue(ctx, mod, ref(vRef))
	length := uint64(len(toSlice(v)))
	mustWriteUint64Le(ctx, mod.Memory(), "length", sp+16, length)
}

// valuePrepareString implements js.valuePrepareString, which is used to load
// the string for `obString()` (via js.jsString) for string, boolean and
// number types.
//
// See https://github.com/golang/go/blob/go1.19rc2/src/syscall/js/js.go#L531
//     https://github.com/golang/go/blob/go1.19rc2/misc/wasm/wasm_exec.js#L402-L405
func valuePrepareString(ctx context.Context, mod api.Module, sp uint32) {
	vRef := mustReadUint64Le(ctx, mod.Memory(), "vRef", sp+8)

	v := loadValue(ctx, mod, ref(vRef))
	s := valueString(v)
	sAddr := storeRef(ctx, mod, s)
	sLen := uint64(len(s))

	mustWriteUint64Le(ctx, mod.Memory(), "sAddr", sp+16, sAddr)
	mustWriteUint64Le(ctx, mod.Memory(), "sLen", sp+24, sLen)
}

// valueLoadString implements js.valueLoadString, which is used copy a string
// value for `obString()`.
//
// See https://github.com/golang/go/blob/go1.19rc2/src/syscall/js/js.go#L533
//     https://github.com/golang/go/blob/go1.19rc2/misc/wasm/wasm_exec.js#L410-L412
func valueLoadString(ctx context.Context, mod api.Module, sp uint32) {
	vRef := mustReadUint64Le(ctx, mod.Memory(), "vRef", sp+8)
	bAddr := mustReadUint64Le(ctx, mod.Memory(), "bAddr", sp+16)
	bLen := mustReadUint64Le(ctx, mod.Memory(), "bLen", sp+24)

	v := loadValue(ctx, mod, ref(vRef))
	s := valueString(v)
	b := mustRead(ctx, mod.Memory(), "b", uint32(bAddr), uint32(bLen))
	copy(b, s)
}

// valueInstanceOf implements js.valueInstanceOf. ex. `array instanceof String`.
//
// See https://github.com/golang/go/blob/go1.19rc2/src/syscall/js/js.go#L543
//     https://github.com/golang/go/blob/go1.19rc2/misc/wasm/wasm_exec.js#L417-L418
func valueInstanceOf(ctx context.Context, mod api.Module, sp uint32) {
	vRef := mustReadUint64Le(ctx, mod.Memory(), "vRef", sp+8)
	tRef := mustReadUint64Le(ctx, mod.Memory(), "tRef", sp+16)

	v := loadValue(ctx, mod, ref(vRef))
	t := loadValue(ctx, mod, ref(tRef))
	var r uint64
	if !instanceOf(v, t) {
		r = 1
	}

	mustWriteUint64Le(ctx, mod.Memory(), "r", sp+24, r)
}

// copyBytesToGo implements js.copyBytesToGo.
//
// Results
//
//	* n is the count of bytes written.
//	* ok is false if the src was not a uint8Array.
//
// See https://github.com/golang/go/blob/go1.19rc2/src/syscall/js/js.go#L569
//     https://github.com/golang/go/blob/go1.19rc2/misc/wasm/wasm_exec.js#L424-L433
func copyBytesToGo(ctx context.Context, mod api.Module, sp uint32) {
	dstAddr := mustReadUint64Le(ctx, mod.Memory(), "dstAddr", sp+8)
	dstLen := mustReadUint64Le(ctx, mod.Memory(), "dstLen", sp+16)
	srcRef := mustReadUint64Le(ctx, mod.Memory(), "srcRef", sp+32)

	dst := mustRead(ctx, mod.Memory(), "dst", uint32(dstAddr), uint32(dstLen)) // nolint
	v := loadValue(ctx, mod, ref(srcRef))
	var n, ok uint64
	if src, isBuf := maybeBuf(v); isBuf {
		n = uint64(copy(dst, src))
		ok = 1
	}

	mustWriteUint64Le(ctx, mod.Memory(), "n", sp+40, n)
	mustWriteUint64Le(ctx, mod.Memory(), "ok", sp+48, ok)
}

// copyBytesToJS implements js.copyBytesToJS.
//
// Results
//
//	* n is the count of bytes written.
//	* ok is false if the dst was not a uint8Array.
//
// See https://github.com/golang/go/blob/go1.19rc2/src/syscall/js/js.go#L583
//     https://github.com/golang/go/blob/go1.19rc2/misc/wasm/wasm_exec.js#L438-L448
func copyBytesToJS(ctx context.Context, mod api.Module, sp uint32) {
	dstRef := mustReadUint64Le(ctx, mod.Memory(), "dstRef", sp+8)
	srcAddr := mustReadUint64Le(ctx, mod.Memory(), "srcAddr", sp+16)
	srcLen := mustReadUint64Le(ctx, mod.Memory(), "srcLen", sp+24)

	src := mustRead(ctx, mod.Memory(), "src", uint32(srcAddr), uint32(srcLen)) // nolint
	v := loadValue(ctx, mod, ref(dstRef))
	var n, ok uint64
	if dst, isBuf := maybeBuf(v); isBuf {
		n = uint64(copy(dst, src))
		ok = 1
	}

	mustWriteUint64Le(ctx, mod.Memory(), "n", sp+40, n)
	mustWriteUint64Le(ctx, mod.Memory(), "ok", sp+48, ok)
}

// refreshSP refreshes the stack pointer, which is needed prior to storeValue
// when in an operation that can trigger a Go event handler.
//
// See https://github.com/golang/go/blob/go1.19rc2/misc/wasm/wasm_exec.js#L210-L213
func refreshSP(mod api.Module) uint32 {
	// Cheat by reading global[0] directly instead of through a function proxy.
	// https://github.com/golang/go/blob/go1.19rc2/src/runtime/rt0_js_wasm.s#L87-L90
	return uint32(mod.(*wasm.CallContext).GlobalVal(0))
}

// syscallFstat is like syscall.Fstat
func syscallFstat(ctx context.Context, mod api.Module, fd uint32) (*jsSt, error) {
	fsc := mod.(*wasm.CallContext).Sys.FS(ctx)
	if f, ok := fsc.OpenedFile(ctx, fd); !ok {
		return nil, errorBadFD(fd)
	} else if stat, err := f.File.Stat(); err != nil {
		return nil, err
	} else {
		return &jsSt{
			isDir:   stat.IsDir(),
			dev:     0, // TODO stat.Sys
			ino:     0,
			mode:    uint32(stat.Mode()),
			nlink:   0,
			uid:     0,
			gid:     0,
			rdev:    0,
			size:    uint32(stat.Size()),
			blksize: 0,
			blocks:  0,
			atimeMs: 0,
			mtimeMs: uint32(stat.ModTime().UnixMilli()),
			ctimeMs: 0,
		}, nil
	}
}

func errorBadFD(fd uint32) error {
	return fmt.Errorf("bad file descriptor: %d", fd)
}

// syscallClose is like syscall.Close
func syscallClose(ctx context.Context, mod api.Module, fd uint32) (err error) {
	fsc := mod.(*wasm.CallContext).Sys.FS(ctx)
	if ok := fsc.CloseFile(ctx, fd); !ok {
		err = errorBadFD(fd)
	}
	return
}

// syscallOpen is like syscall.Open
func syscallOpen(ctx context.Context, mod api.Module, name string, flags, perm uint32) (uint32, error) {
	fsc := mod.(*wasm.CallContext).Sys.FS(ctx)
	return fsc.OpenFile(ctx, name)
}

// syscallRead is like syscall.Read
func syscallRead(ctx context.Context, mod api.Module, fd uint32, p []byte) (n uint32, err error) {
	if r := fdReader(ctx, mod, fd); r == nil {
		err = errorBadFD(fd)
	} else if nRead, e := r.Read(p); e == nil || e == io.EOF {
		// fs_js.go cannot parse io.EOF so coerce it to nil.
		// See https://github.com/golang/go/issues/43913
		n = uint32(nRead)
	} else {
		err = e
	}
	return
}

// syscallWrite is like syscall.Write
func syscallWrite(ctx context.Context, mod api.Module, fd uint32, p []byte) (n uint32, err error) {
	if writer := fdWriter(ctx, mod, fd); writer == nil {
		err = errorBadFD(fd)
	} else if nWritten, e := writer.Write(p); e == nil || e == io.EOF {
		// fs_js.go cannot parse io.EOF so coerce it to nil.
		// See https://github.com/golang/go/issues/43913
		n = uint32(nWritten)
	} else {
		err = e
	}
	return
}

const (
	fdStdin = iota
	fdStdout
	fdStderr
)

// fdReader returns a valid reader for the given file descriptor or nil if ErrnoBadf.
func fdReader(ctx context.Context, mod api.Module, fd uint32) io.Reader {
	sysCtx := mod.(*wasm.CallContext).Sys
	if fd == fdStdin {
		return sysCtx.Stdin()
	} else if f, ok := sysCtx.FS(ctx).OpenedFile(ctx, fd); !ok {
		return nil
	} else {
		return f.File
	}
}

// fdWriter returns a valid writer for the given file descriptor or nil if ErrnoBadf.
func fdWriter(ctx context.Context, mod api.Module, fd uint32) io.Writer {
	sysCtx := mod.(*wasm.CallContext).Sys
	switch fd {
	case fdStdout:
		return sysCtx.Stdout()
	case fdStderr:
		return sysCtx.Stderr()
	default:
		// Check to see if the file descriptor is available
		if f, ok := sysCtx.FS(ctx).OpenedFile(ctx, fd); !ok || f.File == nil {
			return nil
			// fs.FS doesn't declare io.Writer, but implementations such as
			// os.File implement it.
		} else if writer, ok := f.File.(io.Writer); !ok {
			return nil
		} else {
			return writer
		}
	}
}

// funcWrapper is the result of go's js.FuncOf ("_makeFuncWrapper" here).
type funcWrapper struct {
	s *state

	// id is managed on the Go side an increments (possibly rolling over).
	id uint32
}
