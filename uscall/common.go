package uscall

/*
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
	"runtime/cgo"
	"unsafe"
)

const (
	AF_INET       = int32(C.AF_INET)
	SOCK_STREAM   = int32(C.SOCK_STREAM)
	EPOLLIN       = uint32(C.EPOLLIN)
	EPOLLOUT      = uint32(C.EPOLLOUT)
	EPOLL_CTL_ADD = int32(C.EPOLL_CTL_ADD)
	EPOLL_CTL_MOD = int32(C.EPOLL_CTL_MOD)
	EPOLL_CTL_DEL = int32(C.EPOLL_CTL_DEL)
	EPOLLERR      = uint32(C.EPOLLERR)
)

type LoopFunc func(unsafe.Pointer) int32

type Epoll_event C.struct_epoll_event

func (e *Epoll_event) SetSocket(fd int32) *Epoll_event {
	*(*C.int)(unsafe.Pointer(&e.data)) = C.int(fd)
	return e
}

func (e *Epoll_event) Socket() int32 {
	return int32(*(*C.int)(unsafe.Pointer(&e.data)))
}

func (e *Epoll_event) SetHandle(h cgo.Handle) *Epoll_event {
	*(*cgo.Handle)(unsafe.Pointer(&e.data)) = h
	return e
}

func (e *Epoll_event) Handle() cgo.Handle {
	return *(*cgo.Handle)(unsafe.Pointer(&e.data))
}

func (e *Epoll_event) Event() uint32 {
	return uint32(e.events)
}

func (e *Epoll_event) SetEvents(event uint32) *Epoll_event {
	e.events = C.uint32_t(event)
	return e
}

type SockAddr C.struct_sockaddr_in

func (sa *SockAddr) SetFamily(family int32) *SockAddr {
	sa.sin_family = C.sa_family_t(family)
	return sa
}

func (sa *SockAddr) SetPort(port uint) *SockAddr {
	sa.sin_port = C.htons(C.uint16_t(port))
	return sa
}

func (sa *SockAddr) SetAddr(addr string) *SockAddr {
	sa.sin_addr.s_addr = C.htonl(C.INADDR_ANY)
	return sa
}

func (sa *SockAddr) AddrLen() uint32 {
	return uint32(unsafe.Sizeof(*sa))
}
