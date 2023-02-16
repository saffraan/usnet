package usnet

import (
	"net"
	"syscall"
	"usnet/uscall"
)

// A TCPListener is a generic network listener for stream-oriented protocols.
//
// Multiple goroutines may invoke methods on a TCPListener simultaneously.
type TCPListener struct {
	lisfd  *fdesc
	poller *netpoller
	addr   *net.TCPAddr
}

func createTCPListener(addr *net.TCPAddr) (net.Listener, error) {
	// TODO: config init

	// create tcp socket
	sockfd, err := uscall.UscallSocket(uscall.AF_INET, uscall.SOCK_STREAM, 0)
	if err != nil {
		return nil, err
	}

	uscall.UscallIoctlNonBio(sockfd, 1)

	// bind address
	caddr := (&uscall.SockAddr{}).SetFamily(uscall.AF_INET).
		SetPort(uint(addr.Port)).SetAddr(addr.IP.String())

	if _, err = uscall.UscallBind(sockfd, caddr, caddr.AddrLen()); err != nil {
		panic(err)
	}

	// listen socket
	if _, err := uscall.UscallListen(sockfd, 1024); err != nil {
		uscall.UscallClose(sockfd)
		return nil, err
	}

	// create poller
	poller, err := createNetPoller()
	if err != nil {
		uscall.UscallClose(sockfd)
		return nil, err
	}

	// rootEvent := (&uscall.Epoll_event{}).SetSocket(sockfd).
	// 	SetEvents(uscall.EPOLLIN)
	// if err := poller.ctl_add(sockfd, rootEvent); err != nil {
	// 	uscall.UscallClose(sockfd)
	// 	return nil, err
	// }

	return &TCPListener{
		poller: poller,
		lisfd: &fdesc{
			fd:         sockfd,
			irqHandler: newIrqHandler(),
			poller:     poller,
		}}, nil
}

// Accept waits for and returns the next connection to the listener.
func (l *TCPListener) Accept() (net.Conn, error) {
	for {
		addr, addrLen := uscall.SockAddr{}, uint32(0)
		if fd, err := uscall.UscallAccept(l.lisfd.fd, &addr, &addrLen); err != nil {
			if err == syscall.EAGAIN {
				if err = l.lisfd.suspend('r'); err == nil {
					continue
				}
			}
			return nil, err
		} else {
			return NewTCPConn(&fdesc{
				fd:         fd,
				irqHandler: newIrqHandler(),
				poller:     l.poller,
			}, &addr), nil
		}
	}
}

// Close closes the listener.
// Any blocked Accept operations will be unblocked and return errors.
func (l *TCPListener) Close() error {
	if l.lisfd != nil {
		l.lisfd.close()
		l.lisfd = nil
	}
	if l.poller != nil {
		l.poller.close()
		l.poller = nil
	}
	return nil
}

// Addr returns the listener's network address.
func (l *TCPListener) Addr() net.Addr {
	return l.addr
}

type TCPConn struct {
	conn
}

func NewTCPConn(fd *fdesc, addr *uscall.SockAddr) *TCPConn {
	return &TCPConn{
		conn: conn{
			fd: fd,
			rCtx: connCtx{
				buffer: newBuffer(8129),
			},
			wCtx: connCtx{
				buffer: newBuffer(8129),
			},
		},
	}
}
