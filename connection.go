package usnet

import (
	"net"
	"sync"
	"syscall"
	"time"
	"usnet/uscall"
)

type connCtx struct {
	l   sync.RWMutex
	seq int64
	*buffer
}

type conn struct {
	fd         *fdesc
	rCtx, wCtx connCtx
	// local addr
	// remote addr
	utrl UscallController
}

func (c *conn) prepare(mode int) error {
	if err := c.fd.prepare(mode); err != nil {
		return err
	}
	return nil
}

// Read reads data from the connection.
// Read can be made to time out and return an error after a fixed
// time limit; see SetDeadline and SetReadDeadline.
func (c *conn) Read(b []byte) (n int, err error) {
	c.fd.incref('r')
	defer c.fd.decref('r')

	if err = c.prepare('r'); err != nil {
		return 0, err
	}

	return c.safeRead(b)
}

func (c *conn) safeRead(b []byte) (n int, err error) {
	ctx := &c.rCtx
	ctx.l.Lock()
	defer ctx.l.Unlock()

	if err = c.fd.isOk('r'); err == nil {
		for buff, next := ctx.buffer, true; next; ctx.seq++ {
			if buff.Len() <= 0 { // First, Fill read buffer if the buffer is empty.
				var nread = 0
				if nread, err = c.read(); err != nil {
					return
				}
				buff.setLen(nread)
				next = false
			}

			if n += buff.Read(b); n == len(b) { // Second , read from the buffer.
				next = false
			}
		}
	}

	return
}

// Write writes data to the connection.
// Write can be made to time out and return an error after a fixed
// time limit; see SetDeadline and SetWriteDeadline.
func (c *conn) Write(b []byte) (clen int, err error) {
	c.fd.incref('w')
	defer c.fd.decref('w')

	if err = c.prepare('w'); err != nil {
		return 0, err
	}

	return c.safeWrite(b)
}

func (c *conn) read() (int, error) {
	iReq := &irq{ih: &connHandler{conn: c}, reg: c.fd, sig: INT_SIG_INPUT}

	c.fd.trap(iReq)         // enter trap
	defer c.fd.untrap(iReq) // leave trap

	if err := c.fd.isOk('r'); err != nil {
		return 0, err
	}

	c.utrl.Serve(iReq)
	err := c.fd.listen(iReq)
	n, _ := iReq.any.(int)
	return n, err
}

func (c *conn) write() (int, error) {
	iReq := &irq{ih: &connHandler{conn: c}, reg: c.fd, sig: INT_SIG_OUTPUT}

	c.fd.trap(iReq)         // enter trap
	defer c.fd.untrap(iReq) // leave trap

	if err := c.fd.isOk('w'); err != nil {
		return 0, err
	}

	c.utrl.Serve(iReq)
	err := c.fd.listen(iReq)
	n, _ := iReq.any.(int)
	return n, err
}

func (c *conn) safeWrite(b []byte) (clen int, err error) {
	ctx := &c.wCtx
	ctx.l.Lock()
	defer ctx.l.Unlock()

	if err = c.fd.isOk('w'); err == nil {
		buff := ctx.buffer.setPos(0).setLen(0) // clean write buffer
		for dataLen := len(b); clen < dataLen && err == nil; ctx.seq++ {
			if len(b) > 0 {
				b = b[buff.Append(b):] // First: append add into write buffer
			}

			var nwrite int
			if nwrite, err = c.write(); nwrite > 0 { // Second: write data
				clen += nwrite
				buff.move(nwrite)
			}
		}
	}

	return
}

// Close closes the connection.
// Any blocked Read or Write operations will be unblocked and return errors.
func (c *conn) Close() error {
	iReq := &irq{ih: &closeHandler{fd: c.fd}}

	c.fd.trap(iReq)
	defer c.fd.untrap(iReq)

	c.utrl.Serve(iReq)
	return c.fd.listen(iReq)
}

// LocalAddr returns the local network address, if known.
func (c *conn) LocalAddr() net.Addr {
	return nil
}

// RemoteAddr returns the remote network address, if known.
func (c *conn) RemoteAddr() net.Addr {
	return nil
}

// SetDeadline sets the read and write deadlines associated
// with the connection. It is equivalent to calling both
// SetReadDeadline and SetWriteDeadline.
//
// A deadline is an absolute time after which I/O operations
// fail instead of blocking. The deadline applies to all future
// and pending I/O, not just the immediately following call to
// Read or Write. After a deadline has been exceeded, the
// connection can be refreshed by setting a deadline in the future.
//
// If the deadline is exceeded a call to Read or Write or to other
// I/O methods will return an error that wraps os.ErrDeadlineExceeded.
// This can be tested using errors.Is(err, os.ErrDeadlineExceeded).
// The error's Timeout method will return true, but note that there
// are other possible errors for which the Timeout method will
// return true even if the deadline has not been exceeded.
//
// An idle timeout can be implemented by repeatedly extending
// the deadline after successful Read or Write calls.
//
// A zero value for t means I/O operations will not time out.
func (c *conn) SetDeadline(t time.Time) error {
	c.SetReadDeadline(t)
	c.SetWriteDeadline(t)
	return nil
}

// SetReadDeadline sets the deadline for future Read calls
// and any currently-blocked Read call.
// A zero value for t means Read will not time out.
func (c *conn) SetReadDeadline(t time.Time) error {
	c.fd.setReadDeadline(t)
	return nil
}

// SetWriteDeadline sets the deadline for future Write calls
// and any currently-blocked Write call.
// Even if write times out, it may return n > 0, indicating that
// some of the data was successfully written.
// A zero value for t means Write will not time out.
func (c *conn) SetWriteDeadline(t time.Time) error {
	c.fd.setWriteDeadline(t)
	return nil
}

// connHandler implement UscallHandler
type connHandler struct {
	*conn
}

func (c *connHandler) Error(iReq *irq, err error) {
	var ref *int64
	var event uint32

	switch iReq.sig {
	case INT_SIG_INPUT:
		{
			ref, event = &c.fd.rwaits, (uscall.EPOLLIN)
		}
	case INT_SIG_OUTPUT:
		{
			ref, event = &c.fd.wwaits, uscall.EPOLLOUT
		}
	default:
	}

	iReq.err = err
	if iReq.retry > 0 {
		c.fd.netpoller_delete_event(ref, event)
	}
	c.fd.interrupt(INT_SRC_POLLER, func(i *irq) bool {
		return i.seq == iReq.seq
	}, false)
}

func (c *connHandler) Handle(iReq *irq) (callback bool) {
	if iReq == nil {
		return
	}

	var ref *int64
	var event uint32
	callback = true

	switch iReq.sig {
	case INT_SIG_INPUT:
		{
			ref, event = &c.fd.rwaits, (uscall.EPOLLIN)
			if err := c.fd.isOk('r'); err != nil {
				iReq.err = err
				goto connHandleEnd
			}

			if nread, err := c.fd.read(c.rCtx.entity); err == syscall.EAGAIN {
				callback = false
			} else {
				iReq.any, iReq.err = nread, err
			}
		}
	case INT_SIG_OUTPUT:
		{
			ref, event = &c.fd.wwaits, uscall.EPOLLOUT
			if err := c.fd.isOk('w'); err != nil {
				iReq.err = err
				goto connHandleEnd
			}

			if nwrite, err := c.fd.write(c.wCtx.CData()); err == syscall.EAGAIN { // Second: write data
				callback = false
			} else {
				iReq.any, iReq.err = nwrite, err
			}
		}
	default:
	}

connHandleEnd:
	if callback {
		if iReq.retry > 0 {
			c.fd.netpoller_delete_event(ref, event)
		}
		c.fd.interrupt(INT_SRC_POLLER, func(i *irq) bool {
			return i.seq == iReq.seq
		}, false)
	} else {
		if iReq.retry == 0 {
			c.fd.netpoller_add_event(ref, event)
		}
		iReq.retry++
	}
	return
}

type closeHandler struct {
	fd *fdesc
}

func (ch *closeHandler) Handle(iReq *irq) bool {
	ch.fd.close()
	ch.fd.interrupt(INT_SRC_POLLER, func(i *irq) bool {
		return true // match all interrupt request.
	}, true)
	return true
}

func (ch *closeHandler) Error(iReq *irq, err error) {}
