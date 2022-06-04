// Package wasm_exec contains imports and state needed by wasm go compiles when
// GOOS=js and GOARCH=wasm.
//
// See /wasm_exec/REFERENCE.md for a deeper dive.
package wasm_exec

import (
	"context"
	"fmt"
	"io"
	"math"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/sys"
)

// Builder configures the "go" imports used by wasm_exec.js for later use via
// Compile or Instantiate.
type Builder interface {
	Build(context.Context, wazero.Runtime) (WasmExec, error)
}

// NewBuilder returns a new Builder.
func NewBuilder() Builder {
	return &builder{}
}

type builder struct{}

// Build implements Builder.Build
func (b *builder) Build(ctx context.Context, r wazero.Runtime) (WasmExec, error) {
	return newWasmExec(r), nil
}

const (
	parameterSp   = "sp"
	functionDebug = "debug"
)

// moduleBuilder returns a new wazero.ModuleBuilder
func moduleBuilder(r wazero.Runtime) wazero.ModuleBuilder {
	return r.NewModuleBuilder("go").
		ExportFunction(functionWasmExit, wasmExit, functionWasmExit, parameterSp).
		ExportFunction(functionWasmWrite, wasmWrite, functionWasmWrite, parameterSp).
		ExportFunction(resetMemoryDataView.Name, resetMemoryDataView).
		ExportFunction(functionNanotime1, nanotime1, functionNanotime1, parameterSp).
		ExportFunction(functionWalltime, walltime, functionWalltime, parameterSp).
		ExportFunction(functionScheduleTimeoutEvent, scheduleTimeoutEvent, functionScheduleTimeoutEvent, parameterSp).
		ExportFunction(functionClearTimeoutEvent, clearTimeoutEvent, functionClearTimeoutEvent, parameterSp).
		ExportFunction(functionGetRandomData, getRandomData, functionGetRandomData, parameterSp).
		ExportFunction(functionFinalizeRef, finalizeRef, functionFinalizeRef, parameterSp).
		ExportFunction(stringVal.Name, stringVal).
		ExportFunction(valueGet.Name, valueGet).
		ExportFunction(functionValueSet, valueSet, functionValueSet, parameterSp).
		ExportFunction(functionValueDelete, valueDelete, functionValueDelete, parameterSp).
		ExportFunction(functionValueIndex, valueIndex, functionValueIndex, parameterSp).
		ExportFunction(functionValueSetIndex, valueSetIndex, functionValueSetIndex, parameterSp).
		ExportFunction(functionValueCall, valueCall, functionValueCall, parameterSp).
		ExportFunction(functionValueInvoke, valueInvoke, functionValueInvoke, parameterSp).
		ExportFunction(functionValueNew, valueNew, functionValueNew, parameterSp).
		ExportFunction(functionValueLength, valueLength, functionValueLength, parameterSp).
		ExportFunction(functionValuePrepareString, valuePrepareString, functionValuePrepareString, parameterSp).
		ExportFunction(functionValueLoadString, valueLoadString, functionValueLoadString, parameterSp).
		ExportFunction(functionValueInstanceOf, valueInstanceOf, functionValueInstanceOf, parameterSp).
		ExportFunction(functionCopyBytesToGo, copyBytesToGo, functionCopyBytesToGo, parameterSp).
		ExportFunction(functionCopyBytesToJS, copyBytesToJS, functionCopyBytesToJS, parameterSp).
		ExportFunction(debug.Name, debug)
}

// debug has unknown use, so stubbed.
//
// See https://github.com/golang/go/blob/go1.19rc2/src/cmd/link/internal/wasm/asm.go#L133-L138
var debug = &wasm.Func{
	ExportNames: []string{functionDebug},
	Name:        functionDebug,
	ParamTypes:  []wasm.ValueType{wasm.ValueTypeI32},
	ParamNames:  []string{parameterSp},
	Code:        &wasm.Code{Body: []byte{wasm.OpcodeEnd}},
}

// reflectGet implements JavaScript's Reflect.get API.
func reflectGet(ctx context.Context, target interface{}, propertyKey string) interface{} { // nolint
	if target == valueGlobal {
		switch propertyKey {
		case "Object":
			return objectConstructor
		case "Array":
			return arrayConstructor
		case "process":
			return jsProcess
		case "fs":
			return jsFS
		case "Uint8Array":
			return uint8ArrayConstructor
		}
	} else if target == getState(ctx) {
		switch propertyKey {
		case "_pendingEvent":
			return target.(*state)._pendingEvent
		}
	} else if target == jsFS {
		switch propertyKey {
		case "constants":
			return jsFSConstants
		}
	} else if target == io.EOF {
		switch propertyKey {
		case "code":
			return "EOF"
		}
	} else if s, ok := target.(*jsSt); ok {
		switch propertyKey {
		case "dev":
			return s.dev
		case "ino":
			return s.ino
		case "mode":
			return s.mode
		case "nlink":
			return s.nlink
		case "uid":
			return s.uid
		case "gid":
			return s.gid
		case "rdev":
			return s.rdev
		case "size":
			return s.size
		case "blksize":
			return s.blksize
		case "blocks":
			return s.blocks
		case "atimeMs":
			return s.atimeMs
		case "mtimeMs":
			return s.mtimeMs
		case "ctimeMs":
			return s.ctimeMs
		}
	} else if target == jsFSConstants {
		switch propertyKey {
		case "O_WRONLY":
			return oWRONLY
		case "O_RDWR":
			return oRDWR
		case "O_CREAT":
			return oCREAT
		case "O_TRUNC":
			return oTRUNC
		case "O_APPEND":
			return oAPPEND
		case "O_EXCL":
			return oEXCL
		}
	} else if e, ok := target.(*event); ok { // syscall_js.handleEvent
		switch propertyKey {
		case "id":
			return e.id
		case "this": // ex fs
			return e.this
		case "args":
			return e.args
		}
	}
	panic(fmt.Errorf("TODO: reflectGet(target=%v, propertyKey=%s)", target, propertyKey))
}

// reflectGetIndex implements JavaScript's Reflect.get API for an index.
func reflectGetIndex(target interface{}, i uint32) interface{} { // nolint
	return toSlice(target)[i]
}

// reflectSet implements JavaScript's Reflect.set API.
func reflectSet(ctx context.Context, target interface{}, propertyKey string, value interface{}) { // nolint
	if target == getState(ctx) {
		switch propertyKey {
		case "_pendingEvent":
			if value == nil { // syscall_js.handleEvent
				target.(*state)._pendingEvent = nil
				return
			}
		}
	} else if e, ok := target.(*event); ok { // syscall_js.handleEvent
		switch propertyKey {
		case "result":
			e.result = value
			return
		}
	}
	panic(fmt.Errorf("TODO: reflectSet(target=%v, propertyKey=%s, value=%v)", target, propertyKey, value))
}

// reflectSetIndex implements JavaScript's Reflect.set API for an index.
func reflectSetIndex(target interface{}, i uint32, value interface{}) { // nolint
	panic(fmt.Errorf("TODO: reflectSetIndex(target=%v, i=%d, value=%v)", target, i, value))
}

// reflectDeleteProperty implements JavaScript's Reflect.deleteProperty API
func reflectDeleteProperty(target interface{}, propertyKey string) { // nolint
	panic(fmt.Errorf("TODO: reflectDeleteProperty(target=%v, propertyKey=%s)", target, propertyKey))
}

// reflectApply implements JavaScript's Reflect.apply API
func reflectApply(
	ctx context.Context,
	mod api.Module,
	target interface{},
	propertyKey interface{},
	argumentsList []interface{},
) (interface{}, error) { // nolint
	if target == getState(ctx) {
		switch propertyKey {
		case "_makeFuncWrapper":
			return &funcWrapper{s: target.(*state), id: uint32(argumentsList[0].(float64))}, nil
		}
	} else if target == jsFS { // fs_js.go js.fsCall
		// * funcWrapper callback is the last parameter
		//   * arg0 is error and up to one result in arg1
		switch propertyKey {
		case "open":
			// jsFD, err := fsCall("open", name, flags, perm)
			name := argumentsList[0].(string)
			flags := toUint32(argumentsList[1]) // flags are derived from constants like oWRONLY
			perm := toUint32(argumentsList[2])
			result := argumentsList[3].(*funcWrapper)

			fd, err := syscallOpen(ctx, mod, name, flags, perm)
			result.call(ctx, mod, jsFS, err, fd) // note: error first

			return nil, nil
		case "fstat":
			// if stat, err := fsCall("fstat", fd); err == nil && stat.Call("isDirectory").Bool()
			fd := toUint32(argumentsList[0])
			result := argumentsList[1].(*funcWrapper)

			stat, err := syscallFstat(ctx, mod, fd)
			result.call(ctx, mod, jsFS, err, stat) // note: error first

			return nil, nil
		case "close":
			// if stat, err := fsCall("fstat", fd); err == nil && stat.Call("isDirectory").Bool()
			fd := toUint32(argumentsList[0])
			result := argumentsList[1].(*funcWrapper)

			err := syscallClose(ctx, mod, fd)
			result.call(ctx, mod, jsFS, err, true) // note: error first

			return nil, nil
		case "read": // syscall.Read, called by src/internal/poll/fd_unix.go poll.Read.
			// n, err := fsCall("read", fd, buf, 0, len(b), nil)
			fd := toUint32(argumentsList[0])
			buf, ok := maybeBuf(argumentsList[1])
			if !ok {
				return nil, fmt.Errorf("arg[1] is %v not a []byte", argumentsList[1])
			}
			offset := toUint32(argumentsList[2])
			byteCount := toUint32(argumentsList[3])
			_ /* unknown */ = argumentsList[4]
			result := argumentsList[5].(*funcWrapper)

			n, err := syscallRead(ctx, mod, fd, buf[offset:offset+byteCount])
			result.call(ctx, mod, jsFS, err, n) // note: error first

			return nil, nil
		case "write":
			// n, err := fsCall("write", fd, buf, 0, len(b), nil)
			fd := toUint32(argumentsList[0])
			buf, ok := maybeBuf(argumentsList[1])
			if !ok {
				return nil, fmt.Errorf("arg[1] is %v not a []byte", argumentsList[1])
			}
			offset := toUint32(argumentsList[2])
			byteCount := toUint32(argumentsList[3])
			_ /* unknown */ = argumentsList[4]
			result := argumentsList[5].(*funcWrapper)

			n, err := syscallWrite(ctx, mod, fd, buf[offset:offset+byteCount])
			result.call(ctx, mod, jsFS, err, n) // note: error first

			return nil, nil
		}
	} else if target == jsProcess {
		switch propertyKey {
		case "cwd":
			// cwd := jsProcess.Call("cwd").String()
			// TODO
		}
	} else if stat, ok := target.(*jsSt); ok {
		switch propertyKey {
		case "isDirectory":
			return stat.isDir, nil
		}
	}
	panic(fmt.Errorf("TODO: reflectApply(target=%v, propertyKey=%v, argumentsList=%v)", target, propertyKey, argumentsList))
}

func toUint32(arg interface{}) uint32 {
	if arg == refValueZero {
		return 0
	} else if u, ok := arg.(uint32); ok {
		return u
	}
	return uint32(arg.(float64))
}

type event struct {
	// funcWrapper.id
	id     uint32
	this   interface{}
	args   []interface{}
	result interface{}
}

func (f *funcWrapper) call(ctx context.Context, mod api.Module, args ...interface{}) interface{} {
	e := &event{
		id:   f.id,
		this: args[0],
		args: args[1:],
	}

	f.s._pendingEvent = e // Note: _pendingEvent reference is cleared during resume!

	if _, err := mod.ExportedFunction("resume").Call(ctx); err != nil {
		if _, ok := err.(*sys.ExitError); ok {
			return nil // allow error-handling to unwind when wasm calls exit due to a panic
		} else {
			panic(err)
		}
	}

	return e.result
}

// reflectConstruct implements JavaScript's Reflect.construct API
func reflectConstruct(target interface{}, argumentsList []interface{}) (interface{}, error) { // nolint
	if target == uint8ArrayConstructor {
		return make([]byte, uint32(argumentsList[0].(float64))), nil
	}
	panic(fmt.Errorf("TODO: reflectConstruct(target=%v, argumentsList=%v)", target, argumentsList))
}

// valueRef returns 8 bytes to represent either the value or a reference to it.
// Any side effects besides memory must be cleaned up on wasmExit.
//
// See https://github.com/golang/go/blob/go1.19rc2/misc/wasm/wasm_exec.js#L135-L183
func storeRef(ctx context.Context, mod api.Module, v interface{}) uint64 { // nolint
	// allow-list because we control all implementations
	if v == nil {
		return uint64(refValueNull)
	} else if b, ok := v.(bool); ok {
		if b {
			return uint64(refValueTrue)
		} else {
			return uint64(refValueFalse)
		}
	} else if jsV, ok := v.(*constVal); ok {
		return uint64(jsV.ref) // constant doesn't need to be stored
	} else if u, ok := v.(uint64); ok {
		return u // float already encoded as a uint64, doesn't need to be stored.
	} else if fn, ok := v.(*funcWrapper); ok {
		id := fn.s.values.increment(v)
		return uint64(valueRef(id, typeFlagFunction))
	} else if _, ok := v.(*event); ok {
		id := getState(ctx).values.increment(v)
		return uint64(valueRef(id, typeFlagFunction))
	} else if _, ok := v.(string); ok {
		id := getState(ctx).values.increment(v)
		return uint64(valueRef(id, typeFlagString))
	} else if _, ok := v.([]interface{}); ok {
		id := getState(ctx).values.increment(&v) // []interface{} is not hashable
		return uint64(valueRef(id, typeFlagObject))
	} else if _, ok := v.([]byte); ok {
		id := getState(ctx).values.increment(&v) // []byte is not hashable
		return uint64(valueRef(id, typeFlagObject))
	} else if v == io.EOF {
		return uint64(refEOF)
	} else if _, ok := v.(*jsSt); ok {
		id := getState(ctx).values.increment(v)
		return uint64(valueRef(id, typeFlagObject))
	} else if ui, ok := v.(uint32); ok {
		if ui == 0 {
			return uint64(refValueZero)
		}
		return api.EncodeF64(float64(ui)) // numbers are encoded as float and passed through as a ref
	}
	panic(fmt.Errorf("TODO: storeRef(%v)", v))
}

type values struct {
	// Below is needed to avoid exhausting the ID namespace finalizeRef reclaims
	// See https://go-review.googlesource.com/c/go/+/203600

	values      []interface{}          // values indexed by ID, nil
	goRefCounts []uint32               // recount pair-indexed with values
	ids         map[interface{}]uint32 // live values
	idPool      []uint32               // reclaimed IDs (values[i] = nil, goRefCounts[i] nil
}

func (j *values) get(id uint32) interface{} {
	return j.values[id-nextID]
}

func (j *values) increment(v interface{}) uint32 {
	id, ok := j.ids[v]
	if !ok {
		if len(j.idPool) == 0 {
			id, j.values, j.goRefCounts = uint32(len(j.values)), append(j.values, v), append(j.goRefCounts, 0)
		} else {
			id, j.idPool = j.idPool[len(j.idPool)-1], j.idPool[:len(j.idPool)-1]
			j.values[id], j.goRefCounts[id] = v, 0
		}
		j.ids[v] = id
	}
	j.goRefCounts[id]++
	return id + nextID
}

func (j *values) decrement(id uint32) {
	id -= nextID
	j.goRefCounts[id]--
	if j.goRefCounts[id] == 0 {
		j.values[id] = nil
		j.idPool = append(j.idPool, id)
	}
}

var NaN = math.NaN()

// loadValue reads up to 8 bytes at the memory offset `addr` to return the
// value written by storeValue.
//
// See https://github.com/golang/go/blob/go1.19rc2/misc/wasm/wasm_exec.js#L122-L133
func loadValue(ctx context.Context, mod api.Module, ref ref) interface{} { // nolint
	switch ref {
	case refValueNaN:
		return NaN
	case refValueZero:
		return uint32(0)
	case refValueNull:
		return nil
	case refValueTrue:
		return true
	case refValueFalse:
		return false
	case refValueGlobal:
		return valueGlobal
	case refJsGo:
		return getState(ctx)
	case refObjectConstructor:
		return objectConstructor
	case refArrayConstructor:
		return arrayConstructor
	case refJsProcess:
		return jsProcess
	case refJsFS:
		return jsFS
	case refJsFSConstants:
		return jsFSConstants
	case refUint8ArrayConstructor:
		return uint8ArrayConstructor
	case refEOF:
		return io.EOF
	default:
		if (ref>>32)&nanHead != nanHead { // numbers are passed through as a ref
			return api.DecodeF64(uint64(ref))
		}
		return getState(ctx).values.get(uint32(ref))
	}
}

// loadSliceOfValues returns a slice of `len` values at the memory offset
// `addr`
//
// See https://github.com/golang/go/blob/go1.19rc2/misc/wasm/wasm_exec.js#L191-L199
func loadSliceOfValues(ctx context.Context, mod api.Module, sliceAddr, sliceLen uint32) []interface{} { // nolint
	result := make([]interface{}, 0, sliceLen)
	for i := uint32(0); i < sliceLen; i++ { // nolint
		iRef := mustReadUint64Le(ctx, mod.Memory(), "iRef", sliceAddr+i*8)
		result = append(result, loadValue(ctx, mod, ref(iRef)))
	}
	return result
}

// valueString returns the string form of JavaScript string, boolean and number types.
func valueString(v interface{}) string { // nolint
	if s, ok := v.(string); ok {
		return s
	}
	panic(fmt.Errorf("TODO: valueString(%v)", v))
}

// instanceOf returns true if the value is of the given type.
func instanceOf(v, t interface{}) bool { // nolint
	panic(fmt.Errorf("TODO: instanceOf(v=%v, t=%v)", v, t))
}

// mustRead is like api.Memory except that it panics if the offset and
// byteCount are out of range.
func mustRead(ctx context.Context, mem api.Memory, fieldName string, offset, byteCount uint32) []byte {
	buf, ok := mem.Read(ctx, offset, byteCount)
	if !ok {
		panic(fmt.Errorf("Memory.Read(ctx, %d, %d) out of range of memory size %d reading %s",
			offset, byteCount, mem.Size(ctx), fieldName))
	}
	return buf
}

// mustReadUint32Le is like api.Memory except that it panics if the offset
// is out of range.
func mustReadUint32Le(ctx context.Context, mem api.Memory, fieldName string, offset uint32) uint32 {
	result, ok := mem.ReadUint32Le(ctx, offset)
	if !ok {
		panic(fmt.Errorf("Memory.ReadUint64Le(ctx, %d) out of range of memory size %d reading %s",
			offset, mem.Size(ctx), fieldName))
	}
	return result
}

// mustReadUint64Le is like api.Memory except that it panics if the offset
// is out of range.
func mustReadUint64Le(ctx context.Context, mem api.Memory, fieldName string, offset uint32) uint64 {
	result, ok := mem.ReadUint64Le(ctx, offset)
	if !ok {
		panic(fmt.Errorf("Memory.ReadUint64Le(ctx, %d) out of range of memory size %d reading %s",
			offset, mem.Size(ctx), fieldName))
	}
	return result
}

// mustWrite is like api.Memory except that it panics if the offset
// is out of range.
func mustWrite(ctx context.Context, mem api.Memory, fieldName string, offset uint32, val []byte) {
	if ok := mem.Write(ctx, offset, val); !ok {
		panic(fmt.Errorf("Memory.Write(ctx, %d, %d) out of range of memory size %d writing %s",
			offset, val, mem.Size(ctx), fieldName))
	}
}

// mustWriteUint64Le is like api.Memory except that it panics if the offset
// is out of range.
func mustWriteUint64Le(ctx context.Context, mem api.Memory, fieldName string, offset uint32, val uint64) {
	if ok := mem.WriteUint64Le(ctx, offset, val); !ok {
		panic(fmt.Errorf("Memory.WriteUint64Le(ctx, %d, %d) out of range of memory size %d writing %s",
			offset, val, mem.Size(ctx), fieldName))
	}
}

// jsSt is pre-parsed from fs_js.go setStat to avoid thrashin
type jsSt struct {
	isDir bool

	dev     uint32
	ino     uint32
	mode    uint32
	nlink   uint32
	uid     uint32
	gid     uint32
	rdev    uint32
	size    uint32
	blksize uint32
	blocks  uint32
	atimeMs uint32
	mtimeMs uint32
	ctimeMs uint32
}

func toSlice(v interface{}) []interface{} {
	return (*(v.(*interface{}))).([]interface{})
}

func maybeBuf(v interface{}) ([]byte, bool) {
	if p, ok := v.(*interface{}); ok {
		if b, ok := (*(p)).([]byte); ok {
			return b, true
		}
	}
	return nil, false
}
