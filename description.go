package usnet

import (
	"io"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"usnet/uscall"
)

/*
Deadline Context of file descriptions.

	The fdlCtx  will bind  a timer task to interrupt the pending io when deadline exceeds,
	only if the referance counter of fd is more than zero and the dealine is not zero.
*/
type fdlCtx struct {
	l sync.RWMutex

	deadline time.Time // update in UpdateDeadline
	// when deadline execeeded, lazy modify in DealineExceeded
	// reset in UpdateDeadline
	dead   bool
	dlSeq  int64
	dlFunc func() // update in UpdateDeadline

	dlTimer timer
	// Task is must nil when the deadline is zero.
	// and  is lazy inited in UpdateDeadline and fd.prepare when the fd is refered.
	dlTask task
}

func (fc *fdlCtx) Close() {
	fc.UnbindTimer()
}

// UpdateDeadline: with l locker protect.
func (fc *fdlCtx) UpdateDeadline(d time.Time, fd *fdesc, mode int) {
	fc.l.Lock()
	defer fc.l.Unlock()

	// if equal , nothing to do.
	if d.Equal(fc.deadline) {
		return
	}

	fc.dead, fc.deadline, fc.dlFunc = false, d, fc.deadlineFunc(fd, mode)

	bind := !d.IsZero()
	if mode == 'r' {
		bind = bind && atomic.LoadInt64(&fd.rref) > 0
	} else {
		bind = bind && atomic.LoadInt64(&fd.wref) > 0
	}

	if bind {
		fc.bindTimer(true)
	} else {
		fc.unbindTimer()
	}
}

func (fc *fdlCtx) deadlineFunc(fd *fdesc, mode int) func() {
	fd.irqHandler.RLock()
	defer fd.irqHandler.RUnlock()

	seq := atomic.AddInt64(&fc.dlSeq, 1)
	if mode == 'r' {
		fd.checkStatus(RDL_EXECEEDE, true) // clean dealine status
		return func() {
			m := errorWrapMf(equalMf(INT_SIG_INPUT, nil),
				os.ErrDeadlineExceeded)
			fd.irqHandler.interrupt(INT_SRC_TIMER, m, true, func() {
				if atomic.CompareAndSwapInt64(&fc.dlSeq, seq, seq+1) {
					fd.status |= RDL_EXECEEDE
				}
			})
		}
	} else {
		fd.checkStatus(WDL_EXECEEDE, true) // clean dealine status
		return func() {
			m := errorWrapMf(equalMf(INT_SIG_OUTPUT, nil),
				os.ErrDeadlineExceeded)
			fd.irqHandler.interrupt(INT_SRC_TIMER, m, true, func() {
				if atomic.CompareAndSwapInt64(&fc.dlSeq, seq, seq+1) {
					fd.status |= WDL_EXECEEDE
				}
			})
		}
	}
}

// DealineExceeded: return if the deadline has been exceeded
// and the timer  is enable.
func (fc *fdlCtx) DealineExceeded() (dead bool, enable bool) {
	fc.l.RLock()
	defer fc.l.RUnlock()

	// check if deadline exceeded?
	if fc.dead || (!fc.deadline.IsZero() && !fc.deadline.After(time.Now())) {
		fc.dead = true
	}
	return fc.dead, fc.deadline.IsZero() || fc.dlTask != nil
}

func (fc *fdlCtx) BindTimer() {
	fc.l.Lock()
	defer fc.l.Unlock()

	fc.bindTimer(false)
}

func (fc *fdlCtx) bindTimer(force bool) {
	if fc.dlTimer != nil {
		j := &jobImpl{deadline: fc.deadline, proc: fc.dlFunc}
		if fc.dlTask == nil {
			fc.dlTask = fc.dlTimer.add(j)
		} else if force {
			fc.dlTimer.update(fc.dlTask, j)
		}
	}
}

func (c *fdlCtx) UnbindTimer() {
	c.l.Lock()
	defer c.l.Unlock()

	c.unbindTimer()
}

func (fc *fdlCtx) unbindTimer() {
	if fc.dlTask != nil && fc.dlTimer != nil {
		fc.dlTimer.remove(fc.dlTask)
		fc.dlTask = nil
	}
}

type FD_STATUS = int64

const (
	READABLE FD_STATUS = 1 << iota
	WRITEABLE
	READY
	CONNECTED
	ACCEPTED
	ERROR // socket network error in f-stack
	CLOSED
	RDL_EXECEEDE
	WDL_EXECEEDE
)

// Note: fdesc is not concurrency safe.
type fdesc struct {
	fd int32
	*irqHandler

	status FD_STATUS

	rref, wref     int64
	rwaits, wwaits int64

	rdCtx, wdCtx fdlCtx

	poller *netpoller
	ev     *uscall.Epoll_event

	*irqRegister
}

func (fd *fdesc) FD() int32 {
	return fd.fd
}

func (fd *fdesc) incref(mode int) {
	if mode == 'r' {
		atomic.AddInt64(&fd.rref, 1)
	} else {
		atomic.AddInt64(&fd.wref, 1)
	}
}

func (fd *fdesc) decref(mode int) {
	if mode == 'r' {
		atomic.AddInt64(&fd.rref, -1)
	} else {
		atomic.AddInt64(&fd.wref, -1)
	}
}

func (fd *fdesc) netpoller_add_event(ref *int64, event uint32) error {
	if atomic.AddInt64(ref, 1)-1 == 0 {
		fd.irqHandler.Lock()
		defer fd.irqHandler.Unlock()

		if fd.ev == nil {
			ev := (&uscall.Epoll_event{}).SetEvents(event | uscall.EPOLLERR).
				SetSocket(fd.fd)
			if err := fd.poller.ctl_add(fd, ev); err != nil {
				return err
			}
			fd.ev = ev
		} else {
			fd.ev.SetEvents(fd.ev.Event() | event)
			if err := fd.poller.ctl_modify(fd, fd.ev); err != nil {
				fd.ev.SetEvents(fd.ev.Event() & ^event) // rollback events
				return err
			}
		}
	}

	return nil
}

func (fd *fdesc) netpoller_delete_event(ref *int64, event uint32) error {
	if atomic.AddInt64(ref, -1) == 0 {
		fd.irqHandler.Lock()
		defer fd.irqHandler.Unlock()

		if fd.ev == nil {
			return nil
		}

		// if nev := fd.ev.Event() & ^event; nev == uscall.EPOLLERR {
		if nev := fd.ev.Event() & ^event; fd.status&CLOSED != 0 {
			fd.poller.ctl_delete(fd, fd.ev)
			fd.ev = nil
		} else {
			fd.ev.SetEvents(nev)
			if err := fd.poller.ctl_modify(fd, fd.ev); err != nil {
				fd.ev.SetEvents(fd.ev.Event() & ^nev) // rollback events
				return err
			}
		}
	}

	return nil
}

func (fd *fdesc) isOk(mode int) error {
	if fd.status&(ERROR|CLOSED) != 0 {
		return syscall.EINVAL
	}
	if mode == 'r' && fd.status&RDL_EXECEEDE != 0 {
		return os.ErrDeadlineExceeded
	} else if mode == 'w' && fd.status&WDL_EXECEEDE != 0 {
		return os.ErrDeadlineExceeded
	}
	return nil
}

func (fd *fdesc) checkStatus(notice FD_STATUS, autoClean bool) bool {
	st := fd.status & notice
	if autoClean {
		fd.status &= ^st
	}
	return st != 0
}

func (fd *fdesc) suspend(mode int) error {
	var status FD_STATUS
	var iReq irq

	// runtime.LockOSThread()
	// defer runtime.UnlockOSThread()

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
		if err := fd.isOk(mode); err != nil {
			return err
		} else if ok := fd.checkStatus(status, true); ok {
			return nil
		}

		if err := fd.listen(&iReq); err != nil {
			return err
		}
	}
}

func (fd *fdesc) eofError(n int, err error) error {
	if n == 0 && err == nil {
		return io.EOF
	}
	return err
}

// read: return read length [0, ~)， error
func (fd *fdesc) read(cs *uscall.CSlice) (int, error) {
	nread, err := uscall.UscallReadCSlice(fd.fd, cs)
	if err != nil {
		nread = 0 // Note: set zero
	}
	return nread, fd.eofError(nread, err)
}

// read: write written length [0, ~)， error
func (fd *fdesc) write(cs *uscall.CSlice) (nwrite int, err error) {
	if nwrite, err = uscall.UscallWriteCSlice(fd.fd, cs); err != nil {
		nwrite = 0
	} else if nwrite == 0 {
		err = io.ErrUnexpectedEOF
	}
	return
}

func (fd *fdesc) close() (err error) {
	if fd.status&CLOSED == 0 {
		if fd.ev != nil {
			fd.poller.ctl_delete(fd, fd.ev)
			fd.ev = nil
		}
		_, err = uscall.UscallClose(fd.fd)
		fd.status |= CLOSED

		fd.rdCtx.Close()
		fd.wdCtx.Close()
	}
	return err
}

func (fd *fdesc) setReadDeadline(d time.Time) {
	fd.rdCtx.UpdateDeadline(d, fd, 'r')
}

func (fd *fdesc) setWriteDeadline(d time.Time) {
	fd.wdCtx.UpdateDeadline(d, fd, 'w')
}

func (fd *fdesc) prepare(mode int) error {
	var fdCtx = &fd.wdCtx
	if mode == 'r' {
		fdCtx = &fd.rdCtx
	}

	if dead, enable := fdCtx.DealineExceeded(); dead {
		return os.ErrDeadlineExceeded
	} else if !enable {
		// enable deadline timer
		fdCtx.BindTimer()
	}
	return nil
}
