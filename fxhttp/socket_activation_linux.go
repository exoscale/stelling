//gobuild: linux

package fxhttp

import (
	"fmt"
	"net"

	activation "github.com/coreos/go-systemd/activation"
)

func namedSocketListener(name string) (net.Listener, error) {
	listeners, err := activation.ListenersWithNames()
	if err != nil {
		return nil, fmt.Errorf("cannot retrieve listeners: %w", err)
	}
	namedListeners := listeners[name]
	if len(namedListeners) != 1 {
		return nil, fmt.Errorf("named listener count for %s is %d, expected 1", name, len(namedListeners))
	}
	listener := namedListeners[0]
	return listener, nil
}
