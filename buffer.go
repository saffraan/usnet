package usnet

import (
	"runtime"
	"usnet/uscall"
)

type buffer struct {
	entity   *uscall.CSlice
	shadow   []byte
	pos, len int
}

func newBuffer(cap uint32) (b *buffer) {
	b = new(buffer)
	//  alloc a C.struct_slice, share the memory with buffer.
	b.entity = uscall.AllocCSlice(cap, cap)
	runtime.SetFinalizer(b.entity, func(cs *uscall.CSlice) {
		if cs != nil {
			uscall.FreeCSlice(cs)
		}
	})

	b.shadow = uscall.CSlice2Bytes(b.entity)
	return
}

func (b *buffer) setLen(len int) *buffer {
	b.len = len
	return b
}

func (b *buffer) setPos(pos int) *buffer {
	b.pos = pos
	return b
}

func (b *buffer) move(offset int) *buffer {
	b.len -= offset
	b.pos += offset
	return b
}

func (b *buffer) Len() int {
	return b.len
}

// Note: please don't modify result slice, it is ro.
func (b *buffer) Data() []byte {
	return b.shadow[b.pos : b.pos+b.len]
}

func (b *buffer) Read(dst []byte) (clen int) {
	data := b.shadow[b.pos : b.len+b.pos]
	if clen = copy(dst, data); clen < len(data) {
		b.pos += clen // update the unread data area.
		b.len -= clen
	} else {
		b.pos, b.len = 0, 0 // all  data is read.
	}
	return
}

func (s *buffer) Append(data []byte) (clen int) {
	if len(data) > len(s.shadow)-s.pos-s.len {
		s.tidy() // Fress space is not enough
	}
	clen = s.append(data)
	return
}

func (b *buffer) append(src []byte) (clen int) {
	data := b.shadow[b.pos+b.len:]
	clen = copy(data, src)
	b.len += clen
	return
}

// tidy: tidy the  buffer, If  memove moved , return true
func (s *buffer) tidy() bool {
	if s.pos == 0 {
		return false
	}

	// memove data
	copy(s.shadow, s.shadow[s.pos:s.len+s.pos])
	s.pos = 0
	return true
}
