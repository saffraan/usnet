package usnet

import (
	"net"
	"os"
	"runtime"
	"sync"
	"syscall"
	"usnet/uscall"
)

// A TCPListener is a generic network listener for stream-oriented protocols.
//
// Multiple goroutines may invoke methods on a TCPListener simultaneously.
type TCPListener struct {
	lisfd  *fdesc
	utrl   UscallController
	addr   *net.TCPAddr
	poller *netpoller
}

var initOnce sync.Once

func createTCPListener(addr *net.TCPAddr) (l net.Listener, err error) {
	wait := make(chan struct{})
	go func() {
		/*f-stack use tls to store files description and don't support multi-threads posix api.
		must lock os thread and run posix api in one thread.
		*/
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		var sockfd int32
		var utrl *uscallController
		var poller *netpoller
		defer func() {
			close(wait)
			if utrl != nil {
				utrl.proc()
			} else if sockfd > 0 {
				uscall.UscallClose(sockfd)
			}
		}()

		// config init
		initOnce.Do(func() {
			uscall.UscallInit(os.Args)
		})

		// create tcp socket
		if sockfd, err = uscall.UscallSocket(uscall.AF_INET, uscall.SOCK_STREAM, 0); err != nil {
			return
		} else if err = uscall.UscallSetReusePort(sockfd); err != nil {
			return
		}
		uscall.UscallIoctlNonBio(sockfd, 1)
		// bind address
		caddr := (&uscall.SockAddr{}).SetFamily(uscall.AF_INET).
			SetPort(uint(addr.Port)).SetAddr(addr.IP.String())
		if _, err = uscall.UscallBind(sockfd, caddr, caddr.AddrLen()); err != nil {
			return
		}

		// listen socket
		if _, err = uscall.UscallListen(sockfd, 1024); err != nil {
			return
		}

		// create poller
		if poller, err = createNetPoller(); err != nil {
			return
		}

		utrl = NewUscallController(poller)
		l = &TCPListener{
			utrl:   utrl,
			poller: poller,
			lisfd: &fdesc{
				fd:          sockfd,
				irqHandler:  newIrqHandler(),
				poller:      poller,
				irqRegister: &irqRegister{},
			}}
	}()

	<-wait
	return
}

func (l *TCPListener) create(fd int32) *TCPConn {
	return &TCPConn{
		conn: conn{
			rCtx: connCtx{
				buffer: newBuffer(8192),
			},
			wCtx: connCtx{
				buffer: newBuffer(8192),
			},
			fd: &fdesc{
				fd:          fd,
				poller:      l.poller,
				irqRegister: &irqRegister{},
				irqHandler:  newIrqHandler(),
			},
			utrl: l.utrl,
		},
	}
}

func (l *TCPListener) accept() (*TCPConn, error) {
	iReq := &irq{ih: &acceptHandler{TCPListener: l}, reg: l.lisfd}

	l.lisfd.trap(iReq)
	defer l.lisfd.untrap(iReq)

	if err := l.lisfd.isOk('r'); err != nil {
		return nil, err
	}

	l.utrl.Serve(iReq)
	if err := l.lisfd.listen(iReq); err != nil {
		return nil, err
	}
	return iReq.any.(*TCPConn), nil
}

// Accept waits for and returns the next connection to the listener.
func (l *TCPListener) Accept() (net.Conn, error) {
	return l.accept()
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

type acceptHandler struct {
	*TCPListener
}

func (a *acceptHandler) Error(iReq *irq, err error) {
	iReq.err = err
	if iReq.retry > 0 {
		a.lisfd.netpoller_delete_event(&a.lisfd.rwaits, uscall.EPOLLIN)
	}
	a.lisfd.interrupt(INT_SRC_POLLER, func(i *irq) bool {
		return i.seq == iReq.seq
	}, false)
}

func (a *acceptHandler) Handle(iReq *irq) (callback bool) {
	callback = true
	if err := a.lisfd.isOk('r'); err != nil {
		iReq.err = err
	} else {
		addr, addrLen := uscall.SockAddr{}, uint32(0)
		if fd, err := uscall.UscallAccept(a.lisfd.fd, &addr, &addrLen); err != nil {
			if err == syscall.EAGAIN {
				callback = false
			} else {
				iReq.err = err
			}
		} else {
			uscall.UscallIoctlNonBio(fd, 1)
			iReq.any = a.create(fd)
		}
	}

	if callback {
		if iReq.retry > 0 {
			a.lisfd.netpoller_delete_event(&a.lisfd.rwaits, uscall.EPOLLIN)
		}
		a.lisfd.interrupt(INT_SRC_POLLER, func(i *irq) bool {
			return i.seq == iReq.seq
		}, false)
	} else {
		if iReq.retry == 0 {
			a.lisfd.netpoller_add_event(&a.lisfd.rwaits, uscall.EPOLLIN)
		}
		iReq.retry++
	}
	return
}
