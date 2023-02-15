package uscall

import (
	"testing"
)

func testCaseFloat() int32 {
	rs = rs + 1
	return int32(rs)
}

func TestLoopWrapper(t *testing.T) {
	lp := NewLoopParams()
	lp.BindBegin(func() int32 {
		t.Log("loop begin.")
		return 0
	})
	lp.BindEnd(func() int32 {
		t.Log("loop end.")
		return 0
	})
	lp.BindProc(func() int32 {
		t.Log("processing...")
		return 0
	})

	loop(lp)
}

func TestLoopWrapperEmpty(t *testing.T) {
	lp := NewLoopParams()
	loop(lp)
}

func BenchmarkLoopWrapper(b *testing.B) {
	lp := NewLoopParams()
	lp.BindBegin(func() int32 {
		_ = testCaseFloat()
		return 0
	})
	lp.BindEnd(func() int32 {
		_ = testCaseFloat()
		return 0
	})
	lp.BindProc(func() int32 {
		_ = testCaseFloat()
		return 0
	})

	b.RunParallel(func(p *testing.PB) {
		for p.Next() {
			loop(lp)
		}
	})
}

func TestLoopParams_free(t *testing.T) {
	lp := NewLoopParams()
	lp.BindProc(testCaseFloat)

	tests := []struct {
		name string
		lp   *LoopParams
	}{
		{
			name: "free_nil",
			lp:   &LoopParams{},
		}, {
			name: "free_ok",
			lp:   lp,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.lp.free(&tt.lp.fn)
		})
	}
}
