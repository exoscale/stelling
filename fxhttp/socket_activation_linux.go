// Copyright 2015 CoreOS, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//gobuild: linux

package fxhttp

import (
	"fmt"
	"net"

	"github.com/coreos/go-systemd/activation"
)

// gloval var for the named listeners so we just need to read them once
var namedListeners map[string][]net.Listener

// caches the systemd-activated fds and their names
// Returns the listener associated with the arg
func NamedSocketListener(name string) (net.Listener, error) {
	// due to syscall.CloseOnExec(fd), we have to cache the listeners
	var err error
	if namedListeners == nil {
		namedListeners, err = activation.ListenersWithNames()
	}
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
