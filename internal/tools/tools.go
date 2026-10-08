package tools

import (
	"fmt"
	"math/rand/v2"
	"net"
)

func PortRandom() (port int) {
	for {
		port = rand.IntN(9999-1024) + 1024

		if PortIsBindable(port) {
			break
		}
	}
	return port
}

func PortIsBindable(port int) bool {
	addr := fmt.Sprintf(":%d", port)

	c, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	c.Close()

	return true
}
