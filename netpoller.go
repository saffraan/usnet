package usnet

import (
	"errors"
	"sync"
	"syscall"
	"unsafe"
	"usnet/uscall"

	"github.com/redresseur/utils/structure"
)

type netpoller struct {
	epfd     int32
	fdIndexs sync.Map
	events   [4096]uscall.Epoll_event
	ref      int64
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
	if _, err := uscall.UscallEpollCtl(p.epfd, uscall.EPOLL_CTL_ADD, fd.FD(), ev); err != nil {
		return err
	}
	p.fdIndexs.Store(fd.FD(), fd)
	p.ref++
	return nil
}

func (p *netpoller) ctl_modify(fd *fdesc, ev *uscall.Epoll_event) error {
	if _, ok := p.fdIndexs.Load(fd.FD()); !ok {
		return errors.New("the specify fd has not been added into poller")
	}
	if _, err := uscall.UscallEpollCtl(p.epfd, uscall.EPOLL_CTL_MOD, fd.FD(), ev); err != nil {
		return err
	}
	return nil
}

func (p *netpoller) ctl_delete(fd *fdesc, ev *uscall.Epoll_event) error {
	if _, ok := p.fdIndexs.LoadAndDelete(fd.FD()); !ok {
		return errors.New("the specify fd has not been added into poller")
	}
	if _, err := uscall.UscallEpollCtl(p.epfd, uscall.EPOLL_CTL_DEL, fd.FD(), ev); err != nil {
		return err
	}
	p.ref--
	return nil
}

func (p *netpoller) wait(timeout int32) ([]uscall.Epoll_event, error) {
	n, err := uscall.UscallEpollWait(p.epfd, &p.events[0], 4096, timeout)
	if err != nil {
		return nil, err
	}
	return p.events[:n], nil
}

func (p *netpoller) getFd(fd int32) *fdesc {
	v, ok := p.fdIndexs.Load(fd)
	if !ok {
		return nil
	}
	return v.(*fdesc)
}

// uscallController implement UscallController
type uscallController struct {
	p       *netpoller
	irQueue *structure.Queue
}

func NewUscallController(p *netpoller) *uscallController {
	return &uscallController{
		p:       p,
		irQueue: structure.NewQueue(1),
	}
}

func (c *uscallController) Serve(iReq *irq) {
	c.irQueue.Push(iReq)
	c.irQueue.SingleUP(false)
}

func (c *uscallController) proc() {
	uscall.UscallRun(func(p unsafe.Pointer) int32 {

		if c.p.ref == 0 {
			<-c.irQueue.Single()
		}
		for v := c.irQueue.Pop(); v != nil; v = c.irQueue.Pop() {
			if iReq, ok := v.(*irq); ok {
				if !iReq.ih.Handle(iReq) {
					iReq.reg.Save(iReq)
				}
			}
		}

		if c.p.ref == 0 {
			return 0
		}

		// wait events
		if events, err := c.p.wait(0); err != nil {
			return -1
		} else {
			for _, ev := range events {
				c.handle(&ev)
			}
		}
		return 0
	}, nil)
}

func (c *uscallController) getReg(ev *uscall.Epoll_event) UscallRegister {
	if v := c.p.getFd(ev.Socket()); v != nil {
		return v
	}
	return nil
}

func (c *uscallController) handle(ev *uscall.Epoll_event) {
	var efd UscallRegister
	if efd = c.getReg(ev); efd == nil {
		return
	}

	if ev.Event()&uscall.EPOLLIN != 0 {
		efd.Range(func(i *irq) (res bool) {
			if res = (i.sig == INT_SIG_INPUT); res {
				if i.ih.Handle(i) {
					efd.Remove(i)
				}
			}
			return
		})
	}

	if ev.Event()&uscall.EPOLLOUT != 0 {
		efd.Range(func(i *irq) (res bool) {
			if res = (i.sig == INT_SIG_OUTPUT); res {
				if i.ih.Handle(i) {
					efd.Remove(i)
				}
			}
			return
		})
	}

	if ev.Event()&uscall.EPOLLERR != 0 {
		efd.Range(func(i *irq) bool {
			i.ih.Error(i, syscall.EINVAL)
			efd.Remove(i)
			return true
		})
	}
}
