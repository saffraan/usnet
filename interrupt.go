package usnet

import (
	"container/list"
	"sync"
)

type INT_SOURCE int

const (
	INT_SRC_NONE  INT_SOURCE = 0
	INT_SRC_TIMER INT_SOURCE = iota + 1
	INT_SRC_POLLER
)

type INT_SIGNAL int64

const (
	INT_SIG_INPUT INT_SIGNAL = iota
	INT_SIG_OUTPUT
	INT_SIG_TIMEOUT
	INT_SIG_EXP
)

// interrupt request
type irq struct {
	src INT_SOURCE // the source of  interrupt signal
	sig INT_SIGNAL // the
	seq int64
	le  *list.Element

	retry int
	err   error // happend error
	any   interface{}
	ih    UscallHandler
	reg   UscallRegister
}

func (i *irq) Error() error {
	return i.err
}

func (i *irq) bind(sig INT_SIGNAL) *irq {
	i.sig = sig
	return i
}

type irqHandler struct {
	sync.Cond
	sync.RWMutex
	irqList list.List
	seq     int64
}

func newIrqHandler() *irqHandler {
	ih := &irqHandler{}
	ih.Cond.L = &ih.RWMutex
	return ih
}

func (in *irqHandler) ctl_add(i *irq) {
	if i.le == nil {
		i.seq = in.seq
		in.seq++
		i.le = in.irqList.PushBack(i)
	}
}

func (in *irqHandler) ctl_delete(i *irq) {
	if i.le != nil {
		in.irqList.Remove(i.le)
		i.le = nil
	}
}

func (in *irqHandler) trap(i *irq) {
	in.Lock()
	in.ctl_add(i)
}

func (in *irqHandler) untrap(i *irq) {
	in.ctl_delete(i)
	in.Unlock()
}

func (in *irqHandler) listen(i *irq) error {
	for i.src == INT_SRC_NONE {
		in.Wait()
	}
	return i.Error()
}

type matchFunc func(*irq) bool

func equalMf(sig INT_SIGNAL, data interface{}) matchFunc {
	return func(i *irq) bool {
		if i.sig == sig {
			i.any = data
			return true
		}
		return false
	}
}

func errorMf(err error) matchFunc {
	return func(i *irq) bool {
		i.err = err
		return true
	}
}

func errorWrapMf(m matchFunc, err error) matchFunc {
	return func(i *irq) bool {
		if m(i) {
			i.err = err
			return true
		}
		return false
	}
}

// send: send the signal, if all is true, trigger all irqs  match the signal in list,
// else tigger the first irq  match the signal . Ops must be fast, fast and fast.
func (in *irqHandler) interrupt(iSrc INT_SOURCE, mf matchFunc, all bool, ops ...func()) {
	in.Lock()
	// defer in.Unlock()

	// must call op before signal broadcast
	for _, op := range ops {
		op()
	}

	for i := in.irqList.Front(); i != nil; i = i.Next() {
		if ii := i.Value.(*irq); mf(ii) {
			ii.src = iSrc
			in.irqList.Remove(i)
			if !all {
				break
			}
		}
	}
	in.Unlock()
	in.Cond.Broadcast()
}

type irqRegister sync.Map

func (ir *irqRegister) Save(i *irq) {
	(*sync.Map)(ir).Store(i, struct{}{})
}

func (ir *irqRegister) Remove(i *irq) {
	(*sync.Map)(ir).Delete(i)
}

func (ir *irqRegister) Range(f func(*irq) bool) {
	(*sync.Map)(ir).Range(func(key, value any) bool {
		if i, ok := key.(*irq); ok {
			return f(i)
		}
		return true
	})
}

/********************************interface define *******************/
type UscallController interface {
	Serve(*irq)
}

type UscallHandler interface {
	Handle(*irq) (again bool)
	Error(*irq, error)
}

type UscallRegister interface {
	Save(*irq)
	Remove(*irq)
	Range(func(*irq) bool)
	FD() int32
}
