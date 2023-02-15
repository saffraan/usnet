package usnet

import (
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"usnet/uscall"
)

type FD_STATUS = int64

const (
	READABLE FD_STATUS = 1 << iota
	WRITEABLE
	READY
	CONNECTED
	ACCEPTED
	ERROR // socket network error in f-stack
	CLOSED
)

// Note: desc is not concurrency safe.
type desc struct {
	fd int32
	*irqHandler

	status         FD_STATUS
	rwaits, wwaits int64

	poller *netpoller
	ev     *uscall.Epoll_event
}

func (d *desc) netpoller_add_event(ref *int64, event uint32) error {
	if atomic.AddInt64(ref, 1)-1 == 0 {
		if d.ev == nil {
			ev := (&uscall.Epoll_event{}).SetEvents(event | uscall.EPOLLERR)
			if err := d.poller.ctl_add(d.fd, ev); err != nil {
				return err
			}
			d.ev = ev
		} else {
			d.ev.SetEvents(d.ev.Event() | event)
			if err := d.poller.ctl_modify(d.fd, d.ev); err != nil {
				d.ev.SetEvents(d.ev.Event() & ^event) // rollback events
				return err
			}
		}
	}

	return nil
}

func (d *desc) netpoller_delete_event(ref *int64, event uint32) error {
	if atomic.AddInt64(ref, -1) == 0 {
		if d.ev == nil {
			return nil
		}

		if nev := d.ev.Event() & ^event; nev == uscall.EPOLLERR {
			d.poller.ctl_delete(d.fd, d.ev)
			d.ev = nil
		} else {
			d.ev.SetEvents(nev)
			if err := d.poller.ctl_modify(d.fd, d.ev); err != nil {
				d.ev.SetEvents(d.ev.Event() & ^nev) // rollback events
				return err
			}
		}
	}

	return nil
}

func (d *desc) checkStatus(notice FD_STATUS, autoClean bool) (bool, error) {
	if d.status&(ERROR|CLOSED) != 0 {
		return false, syscall.EINVAL
	}

	if st := d.status & notice; st == 0 {
		return false, nil
	} else if autoClean {
		d.status &= ^st
	}

	return true, nil
}

func (fd *desc) suspend(mode int) error {
	var status FD_STATUS
	var iReq irq

	// bind the event into netpoller
	if mode == 'r' {
		status = READABLE
		iReq.bind(INT_SIG_INPUT)
		if err := fd.netpoller_add_event(&fd.rwaits, uscall.EPOLLIN); err != nil { //  add readable event
			return err
		}
		defer fd.netpoller_delete_event(&fd.rwaits, uscall.EPOLLIN) //  remove readable event
	} else {
		status = WRITEABLE
		iReq.bind(INT_SIG_OUTPUT)
		if err := fd.netpoller_add_event(&fd.wwaits, uscall.EPOLLOUT); err != nil { //  add writeable event
			return err
		}
		defer fd.netpoller_delete_event(&fd.wwaits, uscall.EPOLLOUT) // remove writeable event
	}

	fd.trap(&iReq)         // enter trap
	defer fd.untrap(&iReq) // leave trap

	for {
		if ok, err := fd.checkStatus(status, true); ok || err != nil {
			return err
		}

		if err := fd.listen(&iReq); err != nil {
			return err
		}
	}
}

func (fd *desc) eofError(n int, err error) error {
	if n == 0 && err == nil {
		return io.EOF
	}
	return err
}

// read: return read length [0, ~)， error
func (fd *desc) read(cs *uscall.CSlice) (int, error) {
	for {
		nread, err := uscall.UscallReadCSlice(fd.fd, cs)
		if err != nil {
			nread = 0                  // Note: set zero
			if err == syscall.EAGAIN { // try again
				if err = fd.suspend('r'); err == nil { // enter block or wait deadline.
					continue
				}
			}
		}
		return nread, fd.eofError(nread, err)
	}
}

// read: write written length [0, ~)， error
func (fd *desc) write(cs *uscall.CSlice) (int, error) {
	for {
		if nwrite, err := uscall.UscallWriteCSlice(fd.fd, cs); nwrite > 0 {
			return nwrite, err
		} else if err != nil {
			if err == syscall.EAGAIN { // try again
				// enter block or wait deadline.
				if err = fd.suspend('w'); err == nil {
					continue
				}
			}
			return 0, err
		} else if nwrite == 0 {
			return 0, io.ErrUnexpectedEOF
		}
	}
}

func (fd *desc) close() (err error) {
	if fd.status&CLOSED == 0 {
		if fd.ev != nil {
			fd.poller.ctl_delete(fd.fd, fd.ev)
			fd.ev = nil
		}
		_, err = uscall.UscallClose(fd.fd)
		fd.status |= CLOSED
	}
	return err
}

type connCtx struct {
	l   sync.RWMutex
	seq int64
	*buffer

	ld       sync.RWMutex
	deadline time.Time
	dead     bool
	done     func()

	t  timer
	tj task
}

func (c *connCtx) Close() {
	c.UnbindTimer()
}

// UpdateDeadline: with l locker
func (c *connCtx) UpdateDeadline(d time.Time, done func(), bind bool) {
	c.ld.Lock()
	defer c.ld.Unlock()

	// if equal , nothing to do.
	if d.Equal(c.deadline) {
		return
	}

	c.dead, c.deadline, c.done = false, d, done
	if d.IsZero() { // if zero, no deadline
		c.unbindTimer() // ubind timer
	} else if bind && !d.IsZero() {
		c.bindTimer(true)
	}
}

// DealineExceeded: return if the deadline has been exceeded
// and the timer  is enable.
func (c *connCtx) DealineExceeded() (dead bool, enable bool) {
	c.ld.RLock()
	defer c.ld.RUnlock()

	// check if deadline exceeded?
	if c.dead || (!c.deadline.IsZero() && !c.deadline.After(time.Now())) {
		c.dead = true
	}
	return c.dead, c.deadline.IsZero() || c.tj != nil
}

func (c *connCtx) BindTimer() {
	c.ld.Lock()
	defer c.ld.Unlock()

	c.bindTimer(false)
}

func (c *connCtx) bindTimer(force bool) {
	if c.t != nil {
		j := &jobImpl{deadline: c.deadline, proc: c.done}
		if c.tj == nil {
			c.tj = c.t.add(j)
		} else if force {
			c.t.update(c.tj, j)
		}
	}
}

func (c *connCtx) UnbindTimer() {
	c.ld.Lock()
	defer c.ld.Unlock()

	c.unbindTimer()
}

func (c *connCtx) unbindTimer() {
	if c.tj != nil && c.t != nil {
		c.t.remove(c.tj)
		c.tj = nil
	}
}

type conn struct {
	fd         *desc
	rCtx, wCtx connCtx
	// local addr
	// remote addr
}

func (c *conn) prepare(mode int) error {
	var ctx = &c.wCtx
	if mode == 'r' {
		ctx = &c.rCtx
	}

	if dead, enable := ctx.DealineExceeded(); dead {
		return os.ErrDeadlineExceeded
	} else if !enable {
		// enable deadline timer
		ctx.BindTimer()
	}

	return nil
}

// Read reads data from the connection.
// Read can be made to time out and return an error after a fixed
// time limit; see SetDeadline and SetReadDeadline.
func (c *conn) Read(b []byte) (n int, err error) {
	if err = c.prepare('r'); err != nil {
		return 0, err
	}

	return c.safeRead(b)
}

func (c *conn) safeRead(b []byte) (n int, err error) {
	c.rCtx.l.Lock()
	defer c.rCtx.l.Unlock()

	for buff := c.rCtx.buffer; ; c.rCtx.seq++ {
		if buff.Len() <= 0 { // First, Fill read buffer if the buffer is empty.
			var nread = 0
			if nread, err = c.fd.read(buff.entity); err != nil {
				return
			}
			buff.setLen(nread)
		}

		if n += buff.Read(b); n == len(b) { // Second , read from the buffer.
			break
		}
	}

	return
}

// Write writes data to the connection.
// Write can be made to time out and return an error after a fixed
// time limit; see SetDeadline and SetWriteDeadline.
func (c *conn) Write(b []byte) (clen int, err error) {
	if err = c.prepare('w'); err != nil {
		return 0, err
	}

	return c.safeWrite(b)
}

func (c *conn) safeWrite(b []byte) (clen int, err error) {
	c.wCtx.l.Lock()
	defer c.wCtx.l.Unlock()

	buff := c.wCtx.buffer.setPos(0).setLen(0) // clean write buffer
	for dataLen := len(b); clen < dataLen; c.wCtx.seq++ {
		if len(b) > 0 {
			b = b[buff.Append(b):] // First: append add into write buffer
		}

		var nwrite int
		if nwrite, err = c.fd.write(uscall.Bytes2CSlice(buff.Data())); nwrite > 0 { // Second: write data
			clen += nwrite
			buff.move(nwrite)
		}

		if err != nil {
			break
		}
	}

	return
}

// Close closes the connection.
// Any blocked Read or Write operations will be unblocked and return errors.
func (c *conn) Close() error {
	c.rCtx.Close()
	c.wCtx.Close()

	return c.fd.close()
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
	m := errorWrapMf(equalMf(INT_SIG_INPUT, nil),
		os.ErrDeadlineExceeded)
	c.rCtx.UpdateDeadline(t, func() {
		c.fd.interrupt(INT_SRC_TIMER, m, true)
	}, atomic.LoadInt64(&c.fd.rwaits) > 0)

	return nil
}

// SetWriteDeadline sets the deadline for future Write calls
// and any currently-blocked Write call.
// Even if write times out, it may return n > 0, indicating that
// some of the data was successfully written.
// A zero value for t means Write will not time out.
func (c *conn) SetWriteDeadline(t time.Time) error {
	m := errorWrapMf(equalMf(INT_SIG_OUTPUT, nil),
		os.ErrDeadlineExceeded)
	c.wCtx.UpdateDeadline(t, func() {
		c.fd.interrupt(INT_SRC_TIMER, m, true)
	}, atomic.LoadInt64(&c.fd.wwaits) > 0)

	return nil
}
