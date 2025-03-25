//go:build !linux

package fxhttp

import (
	"fmt"
	"net"
)

func NamedSocketListener(name string) (net.Listener, error) {
	return nil, fmt.Errorf("systemd activation is only supported on Linux operating systems")
}
