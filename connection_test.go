//go:build syscall
// +build syscall

package usnet

import (
	"io"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConnRead(t *testing.T) {
	client := testDail(t)
	defer client.Close()

	conn := testNewConn(testAccept(t))
	defer conn.Close()

	input := []byte("data_xxxx")
	go client.Write(input)

	data := make([]byte, 1024)
	n, err := conn.Read(data)
	assert.NoError(t, err)
	assert.Equal(t, input, data[:n])
}

func TestConnWrite(t *testing.T) {
	client := testDail(t)
	conn := testNewConn(testAccept(t))

	input := []byte("data_xxxx")
	_, err := conn.Write(input)
	assert.NoError(t, err, "write failure")

	wait := make(chan struct{})
	output := make([]byte, 1024)
	go func() {
		rlen, err := client.Read(output)
		assert.NoError(t, err, "read failure")
		output = output[:rlen]
		close(wait)
	}()

	<-wait
	assert.NoError(t, err, "write data failure.")
	assert.Equal(t, input, output)
}

func TestConnReadBlock(t *testing.T) {
	client := testDail(t)
	conn := testNewConn(testAccept(t))

	data := []byte("data_xxx")
	go func() {
		time.Sleep(3 * time.Second)
		client.Write(data)
	}()

	output := make([]byte, 1024)
	rlen, err := conn.Read(output)

	assert.NoError(t, err, "read data failure.")
	assert.Equal(t, data, output[:rlen])
}

func TestConnWriteBlock(t *testing.T) {
	client := testDail(t)
	conn := testNewConn(testAccept(t))

	input := []byte("data_xxxx")
	wait := make(chan struct{})
	output := make([]byte, 1024)

	total := 1000000
	go func() {
		time.Sleep(5 * time.Second)
		for rlen := 0; rlen < len(input)*total; {
			n, err := client.Read(output)
			assert.NoError(t, err, "read failure")
			rlen += n
		}

		close(wait)
	}()

	for i := 0; i <= total; i++ {
		_, err := conn.Write(input)
		assert.NoError(t, err, "write failure")
	}

	<-wait
}

func TestConnEcho(t *testing.T) {
	client := testDail(t)
	defer client.Close()

	conn := testNewConn(testAccept(t))
	defer conn.Close()

	go func() {
		buff := make([]byte, 1024)
		for {
			if n, err := client.Read(buff); err != nil {
				break
			} else {
				client.Write(buff[:n])
			}
		}
	}()

	input := testCBytes([]byte("data_xxxx"))
	output := make([]byte, 1024)
	for i := 0; i < 2048; i++ {
		n, err := conn.Write(input)
		assert.NoError(t, err, "write failure")
		assert.Equal(t, len(input), n)

		_, err = conn.Read(output)
		assert.NoError(t, err, "read failure")
		assert.Equal(t, input, output[:n])
	}
}

func TestReadDeadlineException(t *testing.T) {
	client := testDail(t)
	conn := testNewConn(testAccept(t))

	// 1. initial less than now
	conn.SetReadDeadline(time.Now().Add(-1 * time.Second))
	data := make([]byte, 1024)

	_, err := conn.Read(data)
	assert.EqualError(t, err, os.ErrDeadlineExceeded.Error())

	// 2. update less than now
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	go conn.SetReadDeadline(time.Now().Add(-1 * time.Second))
	_, err = conn.Read(data)
	assert.EqualError(t, err, os.ErrDeadlineExceeded.Error())

	// 3. return EOF error
	client.Close()
	conn.SetReadDeadline(time.Time{})
	_, err = conn.Read(data)
	assert.EqualError(t, err, io.EOF.Error())

	// 4. return EINVILA
	conn.Close()
	_, err = conn.Read(data)
	assert.EqualError(t, err, syscall.EINVAL.Error())
}

func TestReadDeadline(t *testing.T) {
	client := testDail(t)
	defer client.Close()

	conn := testNewConn(testAccept(t))
	defer conn.Close()

	// 1. peading and wait dealine
	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	data := make([]byte, 1024)

	_, err := conn.Read(data)
	assert.EqualError(t, err, os.ErrDeadlineExceeded.Error())

	// return immediatly
	_, err = conn.Read(data)
	assert.EqualError(t, err, os.ErrDeadlineExceeded.Error())

	// clean dealine
	conn.SetReadDeadline(time.Time{})

	// 2. update dealine in pending io
	go func() {
		time.Sleep(time.Second)
		conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	}()
	_, err = conn.Read(data)
	assert.EqualError(t, err, os.ErrDeadlineExceeded.Error())

	// return immediatly
	_, err = conn.Read(data)
	assert.EqualError(t, err, os.ErrDeadlineExceeded.Error())

	// 3. update dealine in pending io immediately
	conn.SetReadDeadline(time.Time{})
	go conn.SetReadDeadline(time.Now().Add(time.Second))
	_, err = conn.Read(data)
	assert.EqualError(t, err, os.ErrDeadlineExceeded.Error())

	// 4. very short duration
	conn.SetDeadline(time.Now().Add(time.Millisecond))
	_, err = conn.Read(data)
	assert.EqualError(t, err, os.ErrDeadlineExceeded.Error())

	conn.SetDeadline(time.Time{})
	go conn.SetDeadline(time.Now().Add(time.Millisecond))
	_, err = conn.Read(data)
	assert.EqualError(t, err, os.ErrDeadlineExceeded.Error())
}

func TestReadDeadlineMultiConn(t *testing.T) {
	client := testDail(t)
	defer client.Close()

	conn := testNewConn(testAccept(t))
	defer conn.Close()

	data := make([]byte, 1024)

	// 1. multi-read
	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	wait := make(chan struct{})
	go func() {
		conn.Read(data)
		wait <- struct{}{}
	}()
	_, err := conn.Read(data)
	assert.EqualError(t, err, os.ErrDeadlineExceeded.Error())
	<-wait

	// 2. update deadline
	conn.SetReadDeadline(time.Time{})
	go func() {
		conn.Read(data)
		wait <- struct{}{}
	}()
	go func() {
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	}()
	_, err = conn.Read(data)
	assert.EqualError(t, err, os.ErrDeadlineExceeded.Error())
	<-wait

	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	go func() {
		conn.Read(data)
		wait <- struct{}{}
	}()
	go func() {
		conn.SetReadDeadline(time.Time{})
		time.Sleep(time.Second)
		conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	}()
	_, err = conn.Read(data)
	assert.EqualError(t, err, os.ErrDeadlineExceeded.Error())
	<-wait
}

func testNewConn(fd int32) *conn {
	return &conn{
		fd: testNewDesc(fd),
		rCtx: connCtx{
			buffer: newBuffer(8192),
		},
		wCtx: connCtx{
			buffer: newBuffer(8192),
		},
	}
}
