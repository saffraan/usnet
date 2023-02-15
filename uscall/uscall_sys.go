//go:build syscall
// +build syscall

package uscall

/*
#cgo CFLAGS:  -I/usr/local/include/
#cgo LDFLAGS:  -L/usr/local/lib   -Wl,--whole-archive  -ldpdk  -lfstack  -Wl,--no-whole-archive -lrt -lm -ldl -lcrypto -pthread -lnuma

#include  <stdio.h>
#include <sys/types.h>
#include <sys/socket.h>
#include  <ff_epoll.h>
#include <ff_api.h>
#include <malloc.h>
#include "hook.h"
#include "uscall.h"
#include <arpa/inet.h>
#include <unistd.h>
*/
import "C"
import (
	"syscall"
	"unsafe"
)

/*
the package is main used to test with sycalls.
signal(7) is helpful #    Interruption of system calls and library functions by signal handlers
*/
func UscallEpollWait(epfd int32, events *Epoll_event, maxevents, timeout int32) (int, error) {
	/*
			Interruption of system calls and library functions by stop signals
		       On Linux, even in the absence of signal handlers, certain blocking interfaces can fail with the error EINTR after the process is stopped by one of the stop signals and then resumed via SIGCONT.
		       This behavior is not sanctioned by POSIX.1, and doesn't occur on other systems.
	*/
	for {
		res, err := C.epoll_wait(C.int(epfd), (*C.struct_epoll_event)(events),
			C.int(maxevents), C.int(timeout))
		if res < 0 && err == syscall.EINTR {
			continue
		}
		return int(res), err
	}
}

func UscallRun(loop LoopFunc, arg unsafe.Pointer) {
	lp := NewLoopParams()
	lp.BindProc(func() int32 {
		loop(arg)
		return 0
	})
	C.sys_run_wrap(unsafe.Pointer(lp))
}

func UscallListen(s int32, backlog int32) (int, error) {
	res, err := C.listen(C.int(s), C.int(backlog))
	return int(res), err
}

func UscallBind(s int32, addr *SockAddr, addrLen uint32) (int, error) {
	res, err := C.bind(C.int(s), (*C.struct_sockaddr)(unsafe.Pointer(addr)), C.socklen_t(addrLen))
	return int(res), err
}

func UscallSocket(domain, netType, protocol int32) (int32, error) {
	res, err := C.socket(C.int(domain), C.int(netType), C.int(protocol))
	return int32(res), err
}

func UscallIoctlNonBio(fd int32, on int32) (int32, error) {
	res, err := C.sys_ioctl_non_bio(C.int(fd), C.int(on))
	return int32(res), err
}

func UscallSetReusePort(fd int32) error {
	_, err := C.sys_set_reuse_port(C.int(fd))
	return err
}

func UscallEpollCreate(flag int32) (int, error) {
	res, err := C.epoll_create1(C.int(flag))
	return int(res), err
}

func UscallEpollCtl(epfd, op, fd int32, event *Epoll_event) (int, error) {
	res, err := C.epoll_ctl(C.int(epfd), C.int(op), C.int(fd), (*C.struct_epoll_event)(event))
	return int(res), err
}

func UscallInit(argv []string) (int, error) {
	return 0, nil
}

func UscallAccept(s int32, addr *SockAddr, addrLen *uint32) (int32, error) {
	for {
		res, err := C.accept(C.int(s), (*C.struct_sockaddr)(unsafe.Pointer(addr)),
			(*C.socklen_t)(unsafe.Pointer(addrLen)))
		if res < 0 && err == syscall.EINVAL {
			continue
		}
		return int32(res), err
	}
}

func UscallClose(fd int32) (int32, error) {
	res, err := C.close(C.int(fd))
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
		nread, err := C.sys_read_cslice(C.int(fd), output)
		if !(nread < 0 && err == syscall.EINTR) { // ignore EINTR
			return int(nread), err
		}
	}
}

// UscallReadSlice: Reference Rio
func UscallWriteCSlice(fd int32, input *CSlice) (int, error) {
	for {
		nwrite, err := C.sys_write_cslice(C.int(fd), (*C.struct_slice)(input))
		if !(nwrite <= 0 && err == syscall.EINTR) { // ignore EINTR
			return int(nwrite), err
		}
	}
}
