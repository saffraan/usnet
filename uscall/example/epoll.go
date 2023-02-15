package main

import (
	"os"
	"unsafe"
	"usnet/uscall"
)

func main() {
	uscall.UscallInit(os.Args)
	sockfd, err := uscall.UscallSocket(uscall.AF_INET, uscall.SOCK_STREAM, 0)
	if err != nil {
		panic(err)
	}

	uscall.UscallIoctlNonBio(sockfd, 1)
	myAddr := (&uscall.SockAddr{}).SetFamily(uscall.AF_INET).
		SetPort(8090).SetAddr("0.0.0.0")

	var res int
	res, err = uscall.UscallBind(sockfd, myAddr, myAddr.AddrLen())
	if err != nil {
		panic(err)
	}

	res, err = uscall.UscallListen(sockfd, 512)
	if err != nil {
		panic(err)
	}

	epfd, err := uscall.UscallEpollCreate(0)
	if err != nil {
		panic(err)
	}

	rootEvent := (&uscall.Epoll_event{}).SetSocket(sockfd).SetEvents(uscall.EPOLLIN)
	res, err = uscall.UscallEpollCtl(int32(epfd), uscall.EPOLL_CTL_ADD, sockfd, rootEvent)
	if err != nil {
		panic(err)
	}

	uscall.UscallRun(func(p unsafe.Pointer) {
		loop(int32(epfd), sockfd)
	}, nil)
	_ = res
}

func loop(epfd, socketfd int32) {
	events := make([]uscall.Epoll_event, 512)
	nevents, err := uscall.UscallEpollWait(epfd, &events[0], 512, -1)
	if err != nil {
		panic(err)
	}
	for i := 0; i < nevents; i++ {
		if events[i].Socket() == socketfd {
			accept(epfd, socketfd)
		} else {
			echo(&events[i], epfd)
		}
	}
}

func accept(epfd, socketfd int32) {
	for {
		clientfd, err := uscall.UscallAccept(socketfd, nil, nil)
		if err != nil {
			break
		}

		ev := (&uscall.Epoll_event{}).SetSocket(clientfd).SetEvents(uscall.EPOLLIN)
		_, err = uscall.UscallEpollCtl(epfd, uscall.EPOLL_CTL_ADD, clientfd, ev)
		if err != nil {
			panic(err)
		}
	}
}

var cs = uscall.AllocCSlice(1024, 1024)

func echo(event *uscall.Epoll_event, epfd int32) {
	clientfd := event.Socket()
	if 0 != event.Event()&uint32(uscall.EPOLLERR) {
		uscall.UscallEpollCtl(epfd, uscall.EPOLL_CTL_DEL, clientfd, nil)
		uscall.UscallClose(event.Socket())
	} else {
		s := uscall.CSlice2Bytes(cs)
		if nread, err := uscall.UscallRead(clientfd, s[:]); err != nil || nread == 0 {
			uscall.UscallEpollCtl(epfd, uscall.EPOLL_CTL_DEL, clientfd, nil)
			uscall.UscallClose(clientfd)
			return
		} else {
			uscall.UscallWrite(clientfd, s[:nread])
		}
	}
}
