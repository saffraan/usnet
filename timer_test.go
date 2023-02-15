package usnet

import (
	"testing"
	"time"
)

func BenchmarkAddJob(b *testing.B) {
	tm := NewTimer()
	b.RunParallel(func(p *testing.PB) {
		for p.Next() {
			tm.add(&jobImpl{
				deadline: time.Now().Add(time.Second),
				proc: func() {
					// b.Log("benchmark running")
				},
			})
		}
	})
	b.Log("task number", len(tm.(*timerImpl).hs))
}

func TestAddJob(t *testing.T) {
	tm := NewTimer()
	wait := make(chan struct{})
	tm.add(&jobImpl{
		deadline: time.Now().Add(time.Second),
		proc: func() {
			close(wait)
		},
	})
	<-wait
}

func BenchmarkRemoveJob(b *testing.B) {
	tm := NewTimer()
	b.RunParallel(func(p *testing.PB) {
		for p.Next() {
			tt := tm.add(&jobImpl{
				deadline: time.Now().Add(time.Second),
				proc: func() {
				},
			})
			tm.remove(tt)
		}
	})
	b.Log("task number", len(tm.(*timerImpl).hs))
}

func TestRemoveJob(t *testing.T) {
	tm := NewTimer()
	wait := make(chan struct{})
	tt := tm.add(&jobImpl{
		deadline: time.Now().Add(time.Second),
		proc: func() {
			close(wait)
		},
	})
	tm.remove(tt)
	select {
	case <-wait:
		t.Fatalf("not expected")
	case <-time.After(2 * time.Second):
	}
}

func TestRemoveJobRepeat(t *testing.T) {
	tm := NewTimer()
	wait := make(chan struct{})
	tt := tm.add(&jobImpl{
		deadline: time.Now().Add(time.Second),
		proc: func() {
			close(wait)
		},
	})
	tm.remove(tt)
	tm.remove(tt)
	select {
	case <-wait:
		t.Fatalf("not expected")
	case <-time.After(2 * time.Second):
	}
}

func BenchmarkTestUpdateJob(b *testing.B) {
	tm := NewTimer()
	b.RunParallel(func(p *testing.PB) {
		for p.Next() {
			tt := tm.add(&jobImpl{
				deadline: time.Now().Add(time.Second),
				proc: func() {
				},
			})
			tm.update(tt, &jobImpl{
				deadline: time.Now().Add(time.Microsecond),
				proc: func() {
				},
			})
		}
	})
	b.Log("task number", len(tm.(*timerImpl).hs))
}

func TestUpdateJob(t *testing.T) {
	tm := NewTimer()
	wait := make(chan struct{})
	tt := tm.add(&jobImpl{
		deadline: time.Now().Add(time.Hour),
		proc: func() {
			close(wait)
		},
	})

	tm.update(tt, &jobImpl{
		deadline: time.Now().Add(time.Second),
		proc: func() {
			close(wait)
		},
	})
	<-wait
}

func TestUpdateJobRemove(t *testing.T) {
	tm := NewTimer()
	wait := make(chan struct{})
	tt := tm.add(&jobImpl{
		deadline: time.Now().Add(time.Hour),
		proc: func() {
			close(wait)
		},
	})

	tm.remove(tt)
	tm.update(tt, &jobImpl{
		deadline: time.Now().Add(time.Second),
		proc: func() {
			close(wait)
		},
	})
	<-wait
}
