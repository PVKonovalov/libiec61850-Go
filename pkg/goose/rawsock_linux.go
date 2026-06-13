//go:build linux

/*
 *  rawsock_linux.go
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
	"syscall"
	"unsafe"
)

// linuxRawSocket implements RawSocket using Linux AF_PACKET sockets.
// Requires CAP_NET_RAW capability or root privileges.
type linuxRawSocket struct {
	fd      int
	ifIndex int
}

func newRawSocket(ifaceName string, etherType uint16) (RawSocket, error) {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return nil, fmt.Errorf("interface %q not found: %w", ifaceName, err)
	}

	// ETH_P_ALL = 0x0003; we bind to a specific protocol
	// htons(etherType) for the protocol field
	protocol := int(htons(etherType))

	fd, err := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_RAW, protocol)
	if err != nil {
		return nil, fmt.Errorf("AF_PACKET socket: %w", err)
	}

	sll := syscall.SockaddrLinklayer{
		Protocol: htons(etherType),
		Ifindex:  iface.Index,
	}
	if err := syscall.Bind(fd, &sll); err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("bind to %s: %w", ifaceName, err)
	}

	return &linuxRawSocket{fd: fd, ifIndex: iface.Index}, nil
}

func (s *linuxRawSocket) Send(frame []byte) error {
	addr := &syscall.SockaddrLinklayer{
		Ifindex: s.ifIndex,
		Halen:   6,
	}
	// Destination MAC is the first 6 bytes of the frame
	if len(frame) >= 6 {
		copy(addr.Addr[:6], frame[:6])
	}
	return syscall.Sendto(s.fd, frame, 0, addr)
}

func (s *linuxRawSocket) Close() error {
	return syscall.Close(s.fd)
}

// rawRecvLoop reads frames from the socket and calls handler for each one.
func rawRecvLoop(sock RawSocket, handler func([]byte)) {
	ls := sock.(*linuxRawSocket)
	buf := make([]byte, 4096)
	for {
		n, _, err := syscall.Recvfrom(ls.fd, buf, 0)
		if err != nil {
			return // socket closed
		}
		if n > 0 {
			frame := make([]byte, n)
			copy(frame, buf[:n])
			handler(frame)
		}
	}
}

func getInterfaceMAC(ifaceName string) ([6]byte, error) {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return [6]byte{}, err
	}
	if len(iface.HardwareAddr) < 6 {
		return [6]byte{}, fmt.Errorf("interface %s has no MAC address", ifaceName)
	}
	var mac [6]byte
	copy(mac[:], iface.HardwareAddr[:6])
	return mac, nil
}

func htons(v uint16) uint16 {
	return (v>>8)&0xFF | (v&0xFF)<<8
}

// Ensure we use the unsafe import via a dummy reference.
var _ = unsafe.Sizeof(0)
