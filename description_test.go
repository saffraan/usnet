package usnet

import (
	"fmt"
	"net"
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

func testNewDesc(fd int32) *fdesc {
	testDescInit()
	return &fdesc{
		fd:         fd,
		irqHandler: newIrqHandler(),
		poller:     poller,
		rdCtx: fdlCtx{
			dlTimer: ttimer,
		},
		wdCtx: fdlCtx{
			dlTimer: ttimer,
		},
	}
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

func testAccept(t *testing.T) int32 {
	fd, err := uscall.UscallAccept(sockfd, nil, nil)
	assert.NoError(t, err, "accept  failure.")
	uscall.UscallIoctlNonBio(fd, 1)
	return fd
}

var (
	poller *netpoller
	once   sync.Once
	port   uint = 18090
	addr        = "127.0.0.1"
	sockfd int32
	ttimer = NewTimer()
)
