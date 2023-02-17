//go:build !syscall
// +build !syscall

package uscall

/*
#cgo CFLAGS:  -I/usr/local/include/
#cgo LDFLAGS:  -L/usr/local/lib   -Wl,--whole-archive  -ldpdk  -lfstack  -Wl,--no-whole-archive -lrt -lm -ldl -lcrypto -pthread -lnuma

#include  <stdio.h>
#include <sys/socket.h>
#include  <ff_epoll.h>
#include <ff_api.h>
#include <malloc.h>
#include "hook.h"
#include "uscall.h"
#include <arpa/inet.h>
*/
import "C"
import (
	"syscall"
	"unsafe"
)

func UscallEpollWait(epfd int32, events *Epoll_event, maxevents, timeout int32) (int, error) {
	res, err := C.ff_epoll_wait(C.int(epfd), (*C.struct_epoll_event)(events), C.int(maxevents), C.int(timeout))
	return int(res), err
}

func UscallRun(loop LoopFunc, arg unsafe.Pointer) {
	lp := NewLoopParams()
	lp.BindProc(func() int32 {
		return loop(arg)
	})
	C.ff_run_wrap(unsafe.Pointer(lp))
}

func UscallListen(s int32, backlog int32) (int, error) {
	res, err := C.ff_listen(C.int(s), C.int(backlog))
	return int(res), err
}

func UscallBind(s int32, addr *SockAddr, addrLen uint32) (int, error) {
	res, err := C.ff_bind(C.int(s), (*C.struct_linux_sockaddr)(unsafe.Pointer(addr)), C.socklen_t(addrLen))
	return int(res), err
}

func UscallSocket(domain, netType, protocol int32) (int32, error) {
	res, err := C.ff_socket(C.int(domain), C.int(netType), C.int(protocol))
	return int32(res), err
}

func UscallSetReusePort(fd int32) error {
	return nil
}

func UscallIoctlNonBio(fd int32, on int32) (int32, error) {
	res, err := C.ff_ioctl_non_bio(C.int(fd), C.int(on))
	return int32(res), err
}

func UscallEpollCreate(size int32) (int, error) {
	res, err := C.ff_epoll_create(C.int(size))
	return int(res), err
}

func UscallEpollCtl(epfd, op, fd int32, event *Epoll_event) (int, error) {
	res, err := C.ff_epoll_ctl(C.int(epfd), C.int(op), C.int(fd), (*C.struct_epoll_event)(event))
	return int(res), err
}

func UscallInit(argv []string) (int, error) {
	var args [][]byte
	for _, arg := range argv {
		args = append(args, []byte(arg))
	}

	var b *C.char
	ptrSize := unsafe.Sizeof(b)

	ptr := C.malloc(C.size_t(len(args)) * C.size_t(ptrSize))
	defer func() {
		C.free(ptr)
	}()

	for i := 0; i < len(args); i++ {
		element := (**C.char)(unsafe.Pointer(uintptr(ptr) + uintptr(i)*ptrSize))
		*element = (*C.char)(unsafe.Pointer(&args[i][0]))
	}

	res, err := C.ff_init(C.int(len(args)), (**C.char)(ptr))
	return int(res), err
}

func UscallAccept(s int32, addr *SockAddr, addrLen *uint32) (int32, error) {
	res, err := C.ff_accept(C.int(s), (*C.struct_linux_sockaddr)(unsafe.Pointer(addr)),
		(*C.socklen_t)(unsafe.Pointer(addrLen)))
	return int32(res), err
}

func UscallClose(fd int32) (int32, error) {
	res, err := C.ff_close(C.int(fd))
	return int32(res), err
}

func UscallRead(fd int32, output []byte) (int, error) {
	return UscallReadCSlice(fd, Bytes2CSlice(output))
}

func UscallWrite(fd int32, input []byte) (int, error) {
	return UscallWriteCSlice(fd, Bytes2CSlice(input))
}

// UscallReadSlice: Reference Rio
func UscallReadCSlice(fd int32, output *CSlice) (int, error) {
	for {
		nread, err := C.ff_read_cslice(C.int(fd), output)
		if !(nread < 0 && err == syscall.EINTR) { // ignore EINTR
			return int(nread), err
		}
	}
}

// UscallReadSlice: Reference Rio
func UscallWriteCSlice(fd int32, input *CSlice) (int, error) {
	for {
		nwrite, err := C.ff_write_slice(C.int(fd), (*C.struct_slice)(input))
		if !(nwrite <= 0 && err == syscall.EINTR) { // ignore EINTR
			return int(nwrite), err
		}
	}
}
