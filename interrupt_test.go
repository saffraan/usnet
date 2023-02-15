package usnet

import (
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInterrupt1vs1(t *testing.T) {
	ih := newIrqHandler()
	data := "input_XXXX"

	go func() {
		ih.interrupt(INT_SRC_TEST, equalMf(INT_SIG_INPUT, data), false)
	}()

	call(t, ih, data, nil)
	assert.Equal(t, 0, ih.irqList.Len())
}

func TestInterrupt1vsN(t *testing.T) {
	ih := newIrqHandler()
	data := "input_XXXX"

	n, end, start := 10000, sync.WaitGroup{}, sync.WaitGroup{}
	end.Add(n)
	start.Add(n)

	for i := 0; i < n; i++ {
		go func() {
			start.Done()
			call(t, ih, data, nil)
			end.Done()
		}()
	}

	start.Wait()
	ih.interrupt(INT_SRC_TEST, equalMf(INT_SIG_INPUT, data), true)
	end.Wait()

	assert.Equal(t, 0, ih.irqList.Len())
}

func TestInterrupt1vsNError(t *testing.T) {
	ih := newIrqHandler()
	err := io.EOF

	n, end, start := 10000, sync.WaitGroup{}, sync.WaitGroup{}
	end.Add(n)
	start.Add(n)

	for i := 0; i < n; i++ {
		go func() {
			start.Done()
			call(t, ih, nil, err)
			end.Done()
		}()
	}

	start.Wait()
	ih.interrupt(INT_SRC_TEST, errorMf(err), true)
	end.Wait()

	assert.Equal(t, 0, ih.irqList.Len())
}

func BenchmarkInterrupt(b *testing.B) {
	ih := newIrqHandler()
	data := "input_XXXX"

	for i := 0; i < b.N; i++ {
		stop := false
		go func() {
			ih.interrupt(INT_SRC_TEST, equalMf(INT_SIG_INPUT, data), false)
			stop = true //avoid block
		}()
		call_stop(b, ih, data, nil, &stop)
	}
}

func BenchmarkInterruptParral(b *testing.B) {
	ih := newIrqHandler()
	data := "input_XXXX"

	b.RunParallel(func(p *testing.PB) {
		for p.Next() {
			stop := false
			go func() {
				ih.interrupt(INT_SRC_TEST, equalMf(INT_SIG_INPUT, data), true)
				stop = true //avoid block
			}()
			call_stop(b, ih, data, nil, &stop)
		}
	})
}

const INT_SRC_TEST = iota + 1000

var stop = false

func call(t assert.TestingT, ih *irqHandler, data interface{}, err error) {
	call_stop(t, ih, data, err, &stop)
}

func call_stop(t assert.TestingT, ih *irqHandler, data interface{}, err error, stop *bool) {
	var iReq = (&irq{}).bind(INT_SIG_INPUT)
	ih.trap(iReq)
	defer ih.untrap(iReq)
	for iReq.any == nil && iReq.err == nil && !*stop {
		if !assert.Equal(t, err, ih.listen(iReq)) {
			break
		}
	}
	// t.Logf("the test is end, receive signal: %s", iReq.any)
	if !*stop {
		assert.Equal(t, data, iReq.any)
	}
}
