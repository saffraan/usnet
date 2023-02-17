package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"usnet"
)

var (
	listen    string = "0.0.0.0:8090"
	debug     bool   = false
	processor int    = 2
)

func main() {
	runtime.GOMAXPROCS(processor)
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()
	echo_server()
}

func echo_server() {
	// resolve addr
	l, err := usnet.Listen("tcp", listen)
	if err != nil {
		panic(err)
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			panic(err)
		}

		// fmt.Println("accecpt")
		go func(conn net.Conn) {
			buff := make([]byte, 1024)
			defer conn.Close()
			for {
				if n, err := conn.Read(buff); err != nil {
					if debug {
						fmt.Println(err)
					}
					break
				} else {
					if debug {
						fmt.Println(string(buff[:n]))
					}
					conn.Write(buff[:n])
					if debug {
						fmt.Println()
					}
				}
			}
		}(conn)
	}
}
