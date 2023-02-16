//go:build syscall
// +build syscall

package usnet

import (
	"fmt"
	"net"
	"os"
	"sync"
	"testing"
	"time"
	"usnet/uscall"

	"github.com/stretchr/testify/assert"
)

func TestDescRead(t *testing.T) {
	client := testDail(t)
	desc := testNewDesc(testAccept(t))

	cs := uscall.AllocCSlice(1024, 1024)
	data := []byte("data_xxx")
	go client.Write(data)
	rlen, err := desc.read(cs)

	assert.NoError(t, err, "read data failure.")
	assert.Equal(t, data, uscall.CSlice2Bytes(cs)[:rlen])
}

func TestDescWrite(t *testing.T) {
	client := testDail(t)
	desc := testNewDesc(testAccept(t))

	input := testCBytes([]byte("data_xxxx"))

	_, err := desc.write(uscall.Bytes2CSlice(input))
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

func TestDescReadBlock(t *testing.T) {
	client := testDail(t)
	desc := testNewDesc(testAccept(t))

	cs := uscall.AllocCSlice(1024, 1024)
	data := []byte("data_xxx")

	go func() {
		time.Sleep(3 * time.Second)
		client.Write(data)
	}()

	rlen, err := desc.read(cs)

	assert.NoError(t, err, "read data failure.")
	assert.Equal(t, data, uscall.CSlice2Bytes(cs)[:rlen])
}

func TestDescWriteBlock(t *testing.T) {
	client := testDail(t)
	desc := testNewDesc(testAccept(t))

	input := testCBytes([]byte("data_xxxx"))
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
		_, err := desc.write(uscall.Bytes2CSlice(input))
		assert.NoError(t, err, "write failure")
	}

	<-wait
}

func TestDescEcho(t *testing.T) {
	client := testDail(t)
	defer client.Close()

	desc := testNewDesc(testAccept(t))
	defer desc.close()

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
	output := uscall.AllocCSlice(1024, 1024)
	for i := 0; i < 2048; i++ {
		n, err := desc.write(uscall.Bytes2CSlice(input))
		assert.NoError(t, err, "write failure")
		assert.Equal(t, len(input), n)

		_, err = desc.read(output)
		assert.NoError(t, err, "read failure")
		assert.Equal(t, input, uscall.CSlice2Bytes(output)[:n])
	}
}

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

func TestReadDeadline(t *testing.T) {
	client := testDail(t)
	defer client.Close()

	conn := testNewConn(testAccept(t))
	defer conn.Close()

	// 1. peading and wait dealine
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
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
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	}()
	_, err = conn.Read(data)
	assert.EqualError(t, err, os.ErrDeadlineExceeded.Error())

	// return immediatly
	_, err = conn.Read(data)
	assert.EqualError(t, err, os.ErrDeadlineExceeded.Error())

	// 3. multi-read
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	wait := make(chan struct{})
	go func() {
		conn.Read(data)
		wait <- struct{}{}
	}()
	conn.Read(data)
	<-wait
}

var (
	poller *netpoller
	once   sync.Once
	port   uint = 18090
	addr        = "127.0.0.1"
	sockfd int32
	ttimer = NewTimer()
)

func testCBytes(data []byte) []byte {
	dataLen := uint32(len(data))
	res := uscall.CSlice2Bytes(uscall.AllocCSlice(dataLen, dataLen))
	copy(res, data)
	return res
}

func testDail(t *testing.T) net.Conn {
	testDescInit()

	client, err := net.Dial("tcp", fmt.Sprintf("%s:%d", addr, port))
	assert.NoError(t, err, "connect failure.")
	return client
}

func testDescInit() {
	once.Do(func() {
		var err error
		poller, err = createNetPoller()
		if err != nil {
			panic(err)
		}

		go poller.poll()

		// create tcp socket
		sockfd, err = uscall.UscallSocket(uscall.AF_INET, uscall.SOCK_STREAM, 0)
		if err != nil {
			panic(err)
		}

		if err := uscall.UscallSetReusePort(sockfd); err != nil {
			panic(err)
		}

		uscall.UscallIoctlNonBio(sockfd, 1)

		// bind address
		caddr := (&uscall.SockAddr{}).SetFamily(uscall.AF_INET).
			SetPort(port).SetAddr(addr)

		if _, err = uscall.UscallBind(sockfd, caddr, caddr.AddrLen()); err != nil {
			panic(err)
		}

		// listen socket
		if _, err := uscall.UscallListen(sockfd, 1024); err != nil {
			uscall.UscallClose(sockfd)
			panic(err)
		}
	})
}

func testNewDesc(fd int32) *desc {
	testDescInit()
	return &desc{
		fd:         fd,
		irqHandler: newIrqHandler(),
		poller:     poller,
	}
}

func testNewConn(fd int32) *conn {
	return &conn{
		fd: testNewDesc(fd),
		rCtx: connCtx{
			buffer: newBuffer(8192),
			t:      ttimer,
		},
		wCtx: connCtx{
			buffer: newBuffer(8192),
			t:      ttimer,
		},
	}
}

func testAccept(t *testing.T) int32 {
	fd, err := uscall.UscallAccept(sockfd, nil, nil)
	assert.NoError(t, err, "accept  failure.")
	uscall.UscallIoctlNonBio(fd, 1)
	return fd
}
