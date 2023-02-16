package usnet

import (
	"errors"
	"sync"
	"syscall"
	"usnet/uscall"
)

type netpoller struct {
	epfd     int32
	fdIndexs sync.Map
}

func createNetPoller() (*netpoller, error) {
	epfd, err := uscall.UscallEpollCreate(0)
	if err != nil {
		return nil, err
	}

	return &netpoller{epfd: int32(epfd)}, nil
}

func (p *netpoller) close() {
	if p.epfd >= 0 {
		uscall.UscallClose(p.epfd)
		p.epfd = -1
	}
}

func (p *netpoller) ctl_add(fd *fdesc, ev *uscall.Epoll_event) error {
	if _, err := uscall.UscallEpollCtl(p.epfd, uscall.EPOLL_CTL_ADD, fd.fd, ev); err != nil {
		return err
	}
	p.fdIndexs.Store(fd.fd, fd)
	return nil
}

func (p *netpoller) ctl_modify(fd *fdesc, ev *uscall.Epoll_event) error {
	if _, ok := p.fdIndexs.Load(fd.fd); !ok {
		return errors.New("the specify fd has not been added into poller")
	}
	if _, err := uscall.UscallEpollCtl(p.epfd, uscall.EPOLL_CTL_ADD, fd.fd, ev); err != nil {
		return err
	}
	return nil
}

func (p *netpoller) ctl_delete(fd *fdesc, ev *uscall.Epoll_event) error {
	if _, ok := p.fdIndexs.LoadAndDelete(fd.fd); !ok {
		return errors.New("the specify fd has not been added into poller")
	}
	if _, err := uscall.UscallEpollCtl(p.epfd, uscall.EPOLL_CTL_DEL, fd.fd, ev); err != nil {
		return err
	}
	return nil
}

func (p *netpoller) handle(ev *uscall.Epoll_event) {
	var efd *fdesc
	if fd, ok := p.fdIndexs.Load(ev.Socket()); !ok {
		return
	} else {
		efd = fd.(*fdesc)
	}

	if ev.Event()&uscall.EPOLLIN != 0 {
		efd.irqHandler.interrupt(INT_SRC_POLLER, equalMf(INT_SIG_INPUT, nil), false, func() {
			efd.status |= READABLE
		})
	}

	if ev.Event()&uscall.EPOLLOUT != 0 {
		efd.irqHandler.interrupt(INT_SRC_POLLER, equalMf(INT_SIG_OUTPUT, nil), false, func() {
			efd.status |= WRITEABLE
		})
	}

	if ev.Event()&uscall.EPOLLERR != 0 {
		efd.irqHandler.interrupt(INT_SRC_POLLER, errorMf(syscall.EINVAL), true, func() {
			efd.status |= ERROR
		})
	}
}

func (p *netpoller) poll() {
	for {
		events := make([]uscall.Epoll_event, 4096)
		nevents, err := uscall.UscallEpollWait(p.epfd, &events[0], 4096, -1)
		if err != nil {
			panic(err)
		}

		for i := 0; i < nevents; i++ {
			p.handle(&events[i])
		}
	}
}
