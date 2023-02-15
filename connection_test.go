//go:build syscall
// +build syscall

package usnet

import (
	"fmt"
	"net"
	"sync"
	"testing"
	"usnet/uscall"

	"github.com/stretchr/testify/assert"
)

func TestDescRead(t *testing.T) {
	client := testDail(t)

	fd, err := uscall.UscallAccept(sockfd, nil, nil)
	assert.NoError(t, err, "accept  failure.")

	uscall.UscallIoctlNonBio(fd, 1)
	desc := testNewDesc(fd)

	cs := uscall.AllocCSlice(1024, 1024)
	data := []byte("data_xxx")
	go client.Write(data)
	rlen, err := desc.read(cs)

	assert.NoError(t, err, "read data failure.")
	assert.Equal(t, data, uscall.CSlice2Bytes(cs)[:rlen])
}

func TestDescWrite(t *testing.T) {
	client := testDail(t)

	fd, err := uscall.UscallAccept(sockfd, nil, nil)
	assert.NoError(t, err, "accept  failure.")

	desc := testNewDesc(fd)

	input := testCBytes([]byte("data_xxxx"))

	_, err = desc.write(uscall.Bytes2CSlice(input))
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
	assert.NoError(t, err, "read data failure.")
	assert.Equal(t, input, output)
}

func TestDescReadBlock(t *testing.T) {
	client := testDail(t)

	fd, err := uscall.UscallAccept(sockfd, nil, nil)
	assert.NoError(t, err, "accept  failure.")

	desc := testNewDesc(fd)
	cs := uscall.AllocCSlice(1024, 1024)
	data := []byte("data_xxx")

	// go client.Write(data)
	_ = client
	rlen, err := desc.read(cs)

	assert.NoError(t, err, "read data failure.")
	assert.Equal(t, data, uscall.CSlice2Bytes(cs)[:rlen])
}

var (
	poller *netpoller
	once   sync.Once
	port   uint = 18090
	addr        = "127.0.0.1"
	sockfd int32
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
