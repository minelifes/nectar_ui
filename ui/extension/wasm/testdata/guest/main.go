// A test plugin for package wasm: build with
//
//	GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o guest.wasm .
package main

import (
	"fmt"
	"strconv"
	"unsafe"
)

//go:wasmimport nectar log
func hostLog(ptr unsafe.Pointer, n uint32)

//go:wasmimport nectar notify
func hostNotify(kind uint32, ptr unsafe.Pointer, n uint32) int32

//go:wasmimport nectar register_command
func hostRegister(ptr unsafe.Pointer, n uint32) int32

//go:wasmimport nectar execute_command
func hostExecute(ptr unsafe.Pointer, n uint32) int32

//go:wasmimport nectar storage_get
func hostGet(kptr unsafe.Pointer, klen uint32) uint64

//go:wasmimport nectar storage_set
func hostSet(kptr unsafe.Pointer, klen uint32, vptr unsafe.Pointer, vlen uint32) int32

func ptr(s string) (unsafe.Pointer, uint32) { return unsafe.Pointer(unsafe.StringData(s)), uint32(len(s)) }

func log(s string)                 { p, n := ptr(s); hostLog(p, n) }
func notify(kind uint32, s string) { p, n := ptr(s); hostNotify(kind, p, n) }
func register(id string) int32     { p, n := ptr(id); return hostRegister(p, n) }
func execute(id string) int32      { p, n := ptr(id); return hostExecute(p, n) }

func get(k string) (string, bool) {
	p, n := ptr(k)
	r := hostGet(p, n)
	if r == 0 {
		return "", false
	}
	vp, vn := uint32(r>>32), uint32(r)
	return unsafe.String((*byte)(unsafe.Pointer(uintptr(vp))), vn), true
}

func set(k, v string) int32 {
	kp, kn := ptr(k)
	vp, vn := ptr(v)
	return hostSet(kp, kn, vp, vn)
}

// keep holds the buffers handed to the host.
var keep [][]byte

//go:wasmexport nectar_alloc
func alloc(n uint32) uint32 {
	b := make([]byte, n+1)
	keep = append(keep, b)
	return uint32(uintptr(unsafe.Pointer(unsafe.SliceData(b))))
}

//go:wasmexport activate
func activate() {
	fmt.Println("activating") // stdout goes to the host log
	log("registered: " + strconv.Itoa(int(register("hello.say"))) + " " + strconv.Itoa(int(register("loop.forever"))))
	log("storage: " + strconv.Itoa(int(set("probe", "1"))))
}

//go:wasmexport deactivate
func deactivate() { log("bye") }

//go:wasmexport on_command
func onCommand(p, n uint32) {
	id := unsafe.String((*byte)(unsafe.Pointer(uintptr(p))), n)
	switch id {
	case "hello.say":
		count := 0
		if v, ok := get("count"); ok {
			count, _ = strconv.Atoi(v)
		}
		count++
		set("count", strconv.Itoa(count))
		notify(1, "said hello "+strconv.Itoa(count)+" times")
		execute("app.ping")
	case "loop.forever":
		for {
		}
	}
}

func main() {}
