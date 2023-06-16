//gobuild: linux

package fxhttp

import (
	"fmt"
	"net"
	"sync"

	"github.com/coreos/go-systemd/activation"
)

// gloval var for the named listeners so we just need to read them once
var namedListeners map[string][]net.Listener

// caches the systemd-activated fds and their names
// Returns the listener associated with the arg
func NamedSocketListener(name string) (net.Listener, error) {
	// we cache the listeners because ListenersWithNames call unsets the FDs and
	// I guess it also closes them due to syscall.CloseOnExec
	var err error
	var once sync.Once

	once.Do(func() {
		if namedListeners == nil {
			namedListeners, err = activation.ListenersWithNames()
		}
	})
	if err != nil {
		return nil, err
	}
	namedListeners := namedListeners[name]
	if len(namedListeners) != 1 {
		return nil, fmt.Errorf("named listener count for %s is %d, expected 1", name, len(namedListeners))
	}
	listener := namedListeners[0]
	return listener, nil
}
