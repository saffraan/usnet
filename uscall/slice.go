package uscall

/*
#include "uscall.h"
*/
import "C"
import (
	"reflect"
	"unsafe"
)

type CSlice = C.struct_slice

func Bytes2CSlice(data []byte) *CSlice {
	bh := (*reflect.SliceHeader)(unsafe.Pointer(&data))
	return &C.slice{
		cap: C.uint32_t(bh.Cap),
		len: C.uint32_t(bh.Len),
		ptr: (*C.char)(unsafe.Pointer(bh.Data)),
	}
}

func CSlice2Bytes(cs *CSlice) (data []byte) {
	bh := (*reflect.SliceHeader)(unsafe.Pointer(&data))
	bh.Cap, bh.Len = int(cs.cap), int(cs.len)
	bh.Data = uintptr(unsafe.Pointer(cs.ptr))
	return
}

// alloc: alloc an []byte slice base the memory alloced by C.
func AllocCSlice(len, cap uint32) (cs *CSlice) {
	if cap <= 0 || len > cap {
		panic("alloc out of the memory: len > cap or cap zero.")
	}

	cs = &CSlice{}
	C.slice_alloc_1((*C.struct_slice)(cs), C.uint32_t(cap))
	cs.len = C.uint32_t(len)
	return
}

func FreeCSlice(cs *CSlice) {
	C.slice_free_1(cs)
}
