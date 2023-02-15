package uscall

/*
#include "hook.h"
*/
import "C"
import (
	"runtime"
	"runtime/cgo"
	"unsafe"
)

/*
	To callback golang functions, we define three hooks in C.loop_wrapper function.
	Convert  the type of objects from Handle to cgo.Handle, to run golang functions safety in C stack.
*/

// In cgo, the size of  objects that type is int or long is 4 in C stack.
// Therefore  the type of "Handle"  results should  be int32 in Golang stack.
type Handle func() int32

func hook(handle C.uintptr_t) C.int {
	if handle == 0 {
		return 0
	}

	gh := cgo.Handle(handle).Value()
	return C.int(gh.(Handle)())
}

//export hook_begin
func hook_begin(handle C.uintptr_t) C.int {
	return hook(handle)
}

//export hook_end
func hook_end(handle C.uintptr_t) C.int {
	return hook(handle)
}

//export go_fn_call
func go_fn_call(handle C.uintptr_t) C.int {
	return hook(handle)
}

type LoopParams C.struct_loop_params

// Release: to recycle the cgo.handles, must call.
func NewLoopParams() *LoopParams {
	lp := new(LoopParams)
	runtime.SetFinalizer(lp, func(lp *LoopParams) {
		lp.free(&lp.begin)
		lp.free(&lp.end)
		lp.free(&lp.fn)
	})
	return lp
}

func (lp *LoopParams) free(phandle *C.uintptr_t) {
	if h := *phandle; h > 0 {
		*phandle = 0
		cgo.Handle(h).Delete()
	}
}

func (lp *LoopParams) BindBegin(begin Handle) {
	lp.free(&lp.begin)
	lp.begin = C.uintptr_t(cgo.NewHandle(begin))
}

func (lp *LoopParams) BindEnd(end Handle) {
	lp.free(&lp.end)
	lp.end = C.uintptr_t(cgo.NewHandle(end))
}

func (lp *LoopParams) BindProc(proc Handle) {
	lp.free(&lp.end)
	lp.fn = C.uintptr_t(cgo.NewHandle(proc))
}

// loop: used to test.
func loop(lp *LoopParams) {
	C.loop_wrapper(unsafe.Pointer(lp))
}

var rs C.float
