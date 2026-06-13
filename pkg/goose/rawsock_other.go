//go:build !linux

/*
 *  rawsock_other.go
 *
 *  Copyright 2014-2024 Michael Zillgith
 *  Copyright 2026 Pavel Konovalov Golang port
 *
 *  This file is part of libIEC61850.
 *
 *  libIEC61850 is free software: you can redistribute it and/or modify
 *  it under the terms of the GNU General Public License as published by
 *  the Free Software Foundation, either version 3 of the License, or
 *  (at your option) any later version.
 *
 *  libIEC61850 is distributed in the hope that it will be useful,
 *  but WITHOUT ANY WARRANTY; without even the implied warranty of
 *  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 *  GNU General Public License for more details.
 *
 *  You should have received a copy of the GNU General Public License
 *  along with libIEC61850.  If not, see <http://www.gnu.org/licenses/>.
 *
 *  See COPYING file for the complete license text.
 */

package goose

import (
	"fmt"
	"net"
)

// stubRawSocket is a placeholder for non-Linux platforms.
// GOOSE requires raw Ethernet sockets which need platform-specific implementations.
// On macOS, use BPF (Berkeley Packet Filter) via /dev/bpf*.
// On Windows, use WinPcap/Npcap via golang.org/x/net/bpf.
type stubRawSocket struct {
	closed bool
}

func newRawSocket(ifaceName string, etherType uint16) (RawSocket, error) {
	// Verify the interface exists even on non-Linux so callers get useful errors.
	_, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return nil, fmt.Errorf("interface %q not found: %w", ifaceName, err)
	}
	return &stubRawSocket{}, nil
}

func (s *stubRawSocket) Send(_ []byte) error {
	if s.closed {
		return fmt.Errorf("GOOSE: socket closed")
	}
	return fmt.Errorf("GOOSE: raw socket not supported on this platform; use Linux or implement platform-specific RawSocket")
}

func (s *stubRawSocket) Close() error {
	s.closed = true
	return nil
}

// rawRecvLoop is a no-op on non-Linux platforms.
func rawRecvLoop(_ RawSocket, _ func([]byte)) {}

func getInterfaceMAC(ifaceName string) ([6]byte, error) {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return [6]byte{}, err
	}
	if len(iface.HardwareAddr) < 6 {
		return [6]byte{}, fmt.Errorf("interface %s has no hardware address", ifaceName)
	}
	var mac [6]byte
	copy(mac[:], iface.HardwareAddr)
	return mac, nil
}
