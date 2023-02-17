package usnet

import (
	"container/heap"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type job interface {
	time() time.Time
	run()
}

type jobImpl struct {
	deadline time.Time
	proc     func()
}

func (j *jobImpl) time() time.Time {
	return j.deadline
}

func (j *jobImpl) run() {
	j.proc()
}

type timer interface {
	add(job) task
	remove(task)
	// update: if the task has been execced, re-add the task.
	update(task, job)
	close()
}

type task interface {
	set(job)
}

type Item struct {
	job
	index int
}

func (i *Item) set(j job) {
	i.job = j
}

type heapStore []*Item

func (hs heapStore) root() *Item {
	if len(hs) == 0 {
		return nil
	}

	return hs[0]
}

func (hs heapStore) Len() int { return len(hs) }

func (hs heapStore) Less(i, j int) bool {
	// We want Pop to give us the highest, not lowest, priority so we use greater than here.
	return hs[i].time().After(hs[j].time())
}

func (hs heapStore) Swap(i, j int) {
	hs[i], hs[j] = hs[j], hs[i]
	hs[i].index = i
	hs[j].index = j
}

func (hs *heapStore) Push(x interface{}) {
	n := len(*hs)
	item := x.(*Item)
	item.index = n
	*hs = append(*hs, item)
}

func (hs heapStore) End() *Item {
	return hs[len(hs)-1]
}

func (hs *heapStore) Pop() interface{} {
	old := *hs
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // avoid memory leak
	item.index = -1 // for safety
	*hs = old[0 : n-1]
	return item
}

type timerImpl struct {
	closed chan struct{}

	// heap.heap
	hs heapStore
	l  sync.Mutex

	ignore atomic.Bool
	notify chan struct{}
	ticker *time.Ticker
}

func NewTimer() timer {
	t := &timerImpl{
		hs:     make(heapStore, 0, 1024),
		notify: make(chan struct{}, 1),
		closed: make(chan struct{}),
		ticker: time.NewTicker(time.Hour),
	}
	go t.proc()
	return t
}

func (ti *timerImpl) close() {
	close(ti.closed)
}

func (ti *timerImpl) signal() {
	if !ti.ignore.Load() && len(ti.notify) == 0 {
		select {
		case ti.notify <- struct{}{}:
		default:
		}
	}
}

// find all jobs time  less than now.
func (ti *timerImpl) popJobs(now time.Time) (jobs []job) {
	ti.l.Lock()
	for ti.hs.root() != nil {
		if j := ti.hs.root().job; !j.time().After(now) {
			jobs = append(jobs, j)
			heap.Pop(&ti.hs) // remove from heap
		} else {
			ti.ticker.Reset(j.time().Sub(now))
			break
		}
	}

	if len(ti.hs) == 0 {
		ti.ticker.Reset(time.Hour)
	}

	ti.ignore.Store(len(jobs) != 0) //  avoid no signal when adding new tasks
	ti.l.Unlock()
	return jobs
}

func (ti *timerImpl) proc() {
	var now time.Time
	var jobs []job

	for ok := true; ok; {
		select {
		case <-ti.notify:
			now = time.Now()
		case now = <-ti.ticker.C:
		case <-ti.closed:
			ok, now = false, time.Now() // check all jobs in end.
		}

		ti.ignore.Store(true) // ignore signal

		for {
			if jobs = ti.popJobs(now); len(jobs) == 0 {
				break
			}

			// run jobs
			for _, job := range jobs {
				job.run()
			}
		}

		ti.ignore.Store(false) //  receive signal
	}
}

func (ti *timerImpl) add(j job) task {
	fmt.Println("add job.....")
	ti.l.Lock()
	tt := &Item{job: j}
	heap.Push(&ti.hs, tt)
	ti.l.Unlock()

	ti.signal()
	return tt
}

func (ti *timerImpl) remove(tt task) {
	fmt.Println("remove job.....")
	ti.l.Lock()
	if i, ok := (tt).(*Item); ok {
		if i.index >= 0 && i.index < len(ti.hs) {
			heap.Remove(&ti.hs, i.index)
		}
	}
	ti.l.Unlock()

	ti.signal()
}

func (ti *timerImpl) update(tt task, j job) {
	ti.l.Lock()
	if i, ok := (tt).(*Item); ok && j != nil {
		tt.set(j)
		if i.index >= 0 && i.index < len(ti.hs) {
			heap.Fix(&ti.hs, i.index)
		} else {
			heap.Push(&ti.hs, tt)
		}
	}
	ti.l.Unlock()

	ti.signal()
}
