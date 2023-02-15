package usnet

import "usnet/uscall"

type netpoller struct {
	epfd int32
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

func (p *netpoller) ctl_add(fd int32, ev *uscall.Epoll_event) error {
	_, err := uscall.UscallEpollCtl(p.epfd, uscall.EPOLL_CTL_ADD, fd, ev)
	return err
}

func (p *netpoller) ctl_modify(fd int32, ev *uscall.Epoll_event) error {
	_, err := uscall.UscallEpollCtl(p.epfd, uscall.EPOLL_CTL_ADD, fd, ev)
	return err
}

func (p *netpoller) ctl_delete(fd int32, ev *uscall.Epoll_event) error {
	return nil
}

func (p *netpoller) poll() {
	for {
		events := make([]uscall.Epoll_event, 4096)
		nevents, err := uscall.UscallEpollWait(p.epfd, &events[0], 4096, -1)
		if err != nil {
			panic(err)
		}

		for i := 0; i < nevents; i++ {

		}
	}
}
