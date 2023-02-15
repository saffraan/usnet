package usnet

import (
	"errors"
	"net"
	"strings"
)

func Listen(network, address string) (net.Listener, error) {

	switch strings.ToLower(network) {
	case "tcp", "tcp4", "tcp6":
		{
			// resolve addr
			addr, err := net.ResolveTCPAddr(network, address)
			if err != nil {
				return nil, err
			}

			return createTCPListener(addr)
		}
	default:
		return nil, errors.New(network + "is not supportted now.")
	}
}
