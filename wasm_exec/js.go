package wasm_exec

import (
	"os"

	"github.com/tetratelabs/wazero/api"
)

// ref is used to identify a JavaScript value, since the value itself can not be passed to WebAssembly.
//
// The JavaScript value "undefined" is represented by the value 0.
// A JavaScript number (64-bit float, except 0 and NaN) is represented by its IEEE 754 binary representation.
// All other values are represented as an IEEE 754 binary representation of NaN with bits 0-31 used as
// an ID and bits 32-34 used to differentiate between string, symbol, function and object.
type ref uint64

// nanHead are the upper 32 bits of a ref which are set if the value is not encoded as an IEEE 754 number (see above).
const nanHead = 0x7FF80000

type constVal struct {
	name string
	ref
}

const (
	// the type flags need to be in sync with wasm_exec.js
	typeFlagNone = iota
	typeFlagObject
	typeFlagString
	typeFlagSymbol // nolint
	typeFlagFunction
)

func valueRef(id uint32, typeFlag byte) ref {
	return (nanHead|ref(typeFlag))<<32 | ref(id)
}

const (
	// predefined

	idValueNaN uint32 = iota
	idValueZero
	idValueNull
	idValueTrue
	idValueFalse
	idValueGlobal
	idJsGo

	// The below are derived from analyzing `*_js.go` source.

	idObjectConstructor
	idArrayConstructor
	idJsProcess
	idJsFS
	idJsFSConstants
	idUint8ArrayConstructor
	idJsCrypto
	idJsDateConstructor
	idJsDate
	idEOF
	nextID
)

const (
	refValueNaN    = (nanHead|ref(typeFlagNone))<<32 | ref(idValueNaN)
	refValueZero   = (nanHead|ref(typeFlagNone))<<32 | ref(idValueZero)
	refValueNull   = (nanHead|ref(typeFlagNone))<<32 | ref(idValueNull)
	refValueTrue   = (nanHead|ref(typeFlagNone))<<32 | ref(idValueTrue)
	refValueFalse  = (nanHead|ref(typeFlagNone))<<32 | ref(idValueFalse)
	refValueGlobal = (nanHead|ref(typeFlagObject))<<32 | ref(idValueGlobal)
	refJsGo        = (nanHead|ref(typeFlagObject))<<32 | ref(idJsGo)

	refObjectConstructor     = (nanHead|ref(typeFlagFunction))<<32 | ref(idObjectConstructor)
	refArrayConstructor      = (nanHead|ref(typeFlagFunction))<<32 | ref(idArrayConstructor)
	refJsProcess             = (nanHead|ref(typeFlagObject))<<32 | ref(idJsProcess)
	refJsFS                  = (nanHead|ref(typeFlagObject))<<32 | ref(idJsFS)
	refJsFSConstants         = (nanHead|ref(typeFlagObject))<<32 | ref(idJsFSConstants)
	refUint8ArrayConstructor = (nanHead|ref(typeFlagFunction))<<32 | ref(idUint8ArrayConstructor)
	refJsCrypto              = (nanHead|ref(typeFlagFunction))<<32 | ref(idJsCrypto)
	refJsDateConstructor     = (nanHead|ref(typeFlagFunction))<<32 | ref(idJsDateConstructor)
	refJsDate                = (nanHead|ref(typeFlagObject))<<32 | ref(idJsDate)
	refEOF                   = (nanHead|ref(typeFlagObject))<<32 | ref(idEOF)
)

var (
	// Values below are not built-in, but verifiable by looking at Go's source.
	// When marked "XX.go init", these are eagerly referenced during syscall.init

	// valueGlobal = js.Global() // js.go init
	//
	// Here are its properties:
	//	* js.go
	//	  * objectConstructor = Get("Object") // init
	//	  * arrayConstructor  = Get("Array") // init
	//	* rand_js.go
	//	  * jsCrypto = Get("crypto") // init
	//	  * uint8ArrayConstructor = Get("Uint8Array") // init
	//	* roundtrip_js.go
	//	  * uint8ArrayConstructor = Get("Uint8Array") // init
	//	  * jsFetchMissing = Get("fetch").IsUndefined() // http init
	//	  * Get("AbortController").New() // http.Roundtrip && "fetch"
	//	  * Get("Object").New() // http.Roundtrip && "fetch"
	//	  * Get("Headers").New() // http.Roundtrip && "fetch"
	//	  * Call("fetch", req.URL.String(), opt) && "fetch"
	//	* fs_js.go
	//	  * jsProcess = Get("process") // init
	//	  * jsFS = Get("fs") // init
	//	  * uint8ArrayConstructor = Get("Uint8Array")
	//	* zoneinfo_js.go
	//	  * jsDateConstructor = Get("Date") // time.initLocal
	valueGlobal = &constVal{ref: refValueGlobal, name: "global"}

	// jsGo is not a constant

	// objectConstructor is used by js.ValueOf to make `map[string]any`.
	objectConstructor = &constVal{ref: refObjectConstructor, name: "Object"}

	// arrayConstructor is used by js.ValueOf to make `[]any`.
	arrayConstructor = &constVal{ref: refArrayConstructor, name: "Array"}

	// jsProcess = js.Global().Get("process") // fs_js.go init
	//
	// Here are its properties:
	//	* fs_js.go
	//	  * Call("cwd").String() // fs.Open fs.GetCwd
	//	  * Call("chdir", path) // fs.Chdir
	//	* syscall_js.go
	//	  * Call("getuid").Int() // syscall.Getuid
	//	  * Call("getgid").Int() // syscall.Getgid
	//	  * Call("geteuid").Int() // syscall.Geteuid
	//	  * Call("getgroups") /* array of .Int() */ // syscall.Getgroups
	//	  * Get("pid").Int() // syscall.Getpid
	//	  * Get("ppid").Int() // syscall.Getpid
	//	  * Call("umask", mask /* int */ ).Int() // syscall.Umask
	jsProcess = &constVal{ref: refJsProcess, name: "process"}

	// jsFS = js.Global().Get("fs") // fs_js.go init
	//
	// Here are its properties:
	//	* jsFSConstants = jsFS.Get("constants") // init
	//	* jsFD /* Int */, err := fsCall("open", path, flags, perm) // fs.Open
	//	* stat, err := fsCall("fstat", fd) // fs.Open
	//	  * stat.Call("isDirectory").Bool()
	//	* dir, err := fsCall("readdir", path) // fs.Open
	//	  * dir.Length(), dir.Index(i).String()
	//	* _, err := fsCall("close", fd) // fs.Close
	//	* _, err := fsCall("mkdir", path, perm) // fs.Mkdir
	//	* jsSt, err := fsCall("stat", path) // fs.Stat
	//	* jsSt, err := fsCall("lstat", path) // fs.Lstat
	//	* jsSt, err := fsCall("fstat", fd) // fs.Fstat
	//	* _, err := fsCall("unlink", path) // fs.Unlink
	//	* _, err := fsCall("rmdir", path) // fs.Rmdir
	//	* _, err := fsCall("chmod", path, mode) // fs.Chmod
	//	* _, err := fsCall("fchmod", fd, mode) // fs.Fchmod
	//	* _, err := fsCall("chown", path, uint32(uid), uint32(gid)) // fs.Chown
	//	* _, err := fsCall("fchown", fd, uint32(uid), uint32(gid)) // fs.Fchown
	//	* _, err := fsCall("lchown", path, uint32(uid), uint32(gid)) // fs.Lchown
	//	* _, err := fsCall("utimes", path, atime, mtime) // fs.UtimesNano
	//	* _, err := fsCall("rename", from, to) // fs.Rename
	//	* _, err := fsCall("truncate", path, length) // fs.Truncate
	//	* _, err := fsCall("ftruncate", fd, length) // fs.Ftruncate
	//	* dst, err := fsCall("readlink", path) // fs.Readlink
	//	* _, err := fsCall("link", path, link) // fs.Link
	//	* _, err := fsCall("symlink", path, link) // fs.Symlink
	//	* _, err := fsCall("fsync", fd) // fs.Fsync
	//	* n, err := fsCall("read", fd, buf, 0, len(b), nil) // fs.Read
	//	* n, err := fsCall("write", fd, buf, 0, len(b), nil) // fs.Write
	//	* n, err := fsCall("read", fd, buf, 0, len(b), offset) // fs.Pread
	//	* n, err := fsCall("write", fd, buf, 0, len(b), offset) // fs.Pwrite
	jsFS = &constVal{ref: refJsFS, name: "fs"}

	// jsFSConstants = jsFS Get("constants") // fs_js.go init
	jsFSConstants = &constVal{ref: refJsFSConstants, name: "constants"}

	// oWRONLY = jsFSConstants Get("O_WRONLY").Int() // fs_js.go init
	oWRONLY = api.EncodeF64(float64(os.O_WRONLY))

	// oRDWR = jsFSConstants Get("O_RDWR").Int() // fs_js.go init
	oRDWR = api.EncodeF64(float64(os.O_RDWR))

	//o CREAT = jsFSConstants Get("O_CREAT").Int() // fs_js.go init
	oCREAT = api.EncodeF64(float64(os.O_CREATE))

	// oTRUNC = jsFSConstants Get("O_TRUNC").Int() // fs_js.go init
	oTRUNC = api.EncodeF64(float64(os.O_TRUNC))

	// oAPPEND = jsFSConstants Get("O_APPEND").Int() // fs_js.go init
	oAPPEND = api.EncodeF64(float64(os.O_APPEND))

	// oEXCL = jsFSConstants Get("O_EXCL").Int() // fs_js.go init
	oEXCL = api.EncodeF64(float64(os.O_EXCL))

	// uint8ArrayConstructor = js.Global().Get("Uint8Array")
	//	// fs_js.go, rand_js.go, roundtrip_js.go init
	//
	// It has only one invocation pattern: `buf := uint8Array.New(len(b))`
	uint8ArrayConstructor = &constVal{ref: refUint8ArrayConstructor, name: "Uint8Array"}

	// jsCrypto = js.Global().Get("crypto") // rand_js.go init
	//
	// It has only one invocation pattern:
	//	`jsCrypto.Call("getRandomValues", a /* uint8Array */)`
	_ = /* jsCrypto */ &constVal{ref: refJsCrypto, name: "crypto"}

	// jsDateConstructor is used inline in zoneinfo_js.go for time.initLocal.
	// `New()` returns jsDate.
	_ = /* jsDateConstructor */ &constVal{ref: refJsDateConstructor, name: "Date"}

	// jsDate is used inline in zoneinfo_js.go for time.initLocal.
	// `.Call("getTimezoneOffset").Int()` returns a timezone offset.
	_ = /* jsDate */ &constVal{ref: refJsDate, name: "jsDate"}
)
