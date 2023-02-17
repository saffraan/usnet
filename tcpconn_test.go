package usnet

import (
	"fmt"
	"net"
	"testing"
	"usnet/uscall"

	"github.com/stretchr/testify/assert"
)

func TestListen(t *testing.T) {
	l, err := Listen("tcp", fmt.Sprintf("%s:%d", addr, port))
	assert.NoError(t, err)
	go func() {
		client, err := net.Dial("tcp", fmt.Sprintf("%s:%d", addr, port))
		assert.NoError(t, err)
		client.Close()
	}()
	conn, err := l.Accept()
	assert.NoError(t, err)
	conn.Close()
}

func TestMain(m *testing.M) {
	uscall.UscallInit([]string{"--conf", "config.ini", "--proc-type=primary", "--proc-id=0"})
	m.Run()
}
