package usnet

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAppend(t *testing.T) {
	data := []byte("Hello world!!!")
	s := newBuffer(uint32(2*len(data) - 1))
	if !assert.Equal(t, len(data), s.Append(data)) {
		t.Fail()
	}

	if !assert.Equal(t, len(data)-1, s.Append(data)) {
		t.Fail()
	}
}

func TestRead(t *testing.T) {
	data := []byte("Hello world!!!")
	s := newBuffer(uint32(2*len(data) - 1))

	s.Append(data)
	if !assert.Equal(t, 1, s.Read([]byte{0})) {
		t.Fail()
	}

	if !assert.Equal(t, s.Append(data), len(data)) {
		t.Fail()
	}
}
