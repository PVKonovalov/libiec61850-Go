/*
 *  cotp.go
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

// Package cotp implements the ISO 8073 Connection-Oriented Transport Protocol (COTP)
// over TCP using RFC 1006 encapsulation.
//
// RFC 1006 wraps ISO transport PDUs in a 4-byte TPKT header:
//
//	[0x03][0x00][len_high][len_low]
//
// COTP supports three PDU types used in MMS:
//   - CR (Connection Request,  code 0xE0)
//   - CC (Connection Confirm,  code 0xD0)
//   - DT (Data,                code 0xF0)
package cotp

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"
)

const (
	tpktVersion  = 0x03
	tpktReserved = 0x00

	// TPKT header size: version(1) + reserved(1) + length(2)
	tpktHeaderSize = 4

	// COTP PDU type codes
	pduCR = 0xE0 // Connection Request
	pduCC = 0xD0 // Connection Confirm
	pduDR = 0x80 // Disconnect Request
	pduDC = 0xC0 // Disconnect Confirm
	pduDT = 0xF0 // Data
	pduER = 0x70 // TPDU Error

	// COTP option parameter codes
	optTPDUSize    = 0xC0
	optSrcTSel     = 0xC1
	optDstTSel     = 0xC2
	optMaxTSDUSize = 0xC3 // not used in IEC 61850

	// Default TPDU size code (log2 of max TPDU size)
	defaultTPDUSizeCode = 0x0B // 2048 bytes
)

// TSelector is a transport selector (used in CR/CC to identify endpoints).
type TSelector struct {
	Value []byte
}

// Options holds the connection parameters exchanged in CR/CC PDUs.
type Options struct {
	TPDUSizeCode byte      // 0x07=128, 0x08=256, 0x09=512, 0x0A=1024, 0x0B=2048, 0x0C=4096, 0x0D=8192
	SrcTSel      TSelector // calling transport selector
	DstTSel      TSelector // called transport selector
}

// DefaultOptions returns default COTP options suitable for MMS.
func DefaultOptions() Options {
	return Options{
		TPDUSizeCode: defaultTPDUSizeCode,
		SrcTSel:      TSelector{Value: []byte{0x00, 0x01}},
		DstTSel:      TSelector{Value: []byte{0x00, 0x01}},
	}
}

// Conn wraps a net.Conn and provides COTP framing.
type Conn struct {
	conn      net.Conn
	opts      Options
	localRef  uint16
	remoteRef uint16
}

// NewConn creates a COTP connection over an existing TCP connection.
func NewConn(conn net.Conn, opts Options) *Conn {
	return &Conn{
		conn:     conn,
		opts:     opts,
		localRef: 1,
	}
}

// Connect sends a COTP Connection Request (CR) and waits for Connection Confirm (CC).
func (c *Conn) Connect() error {
	cr := c.buildCR()
	if err := c.sendRaw(cr); err != nil {
		return fmt.Errorf("COTP: send CR: %w", err)
	}
	buf, err := c.readTPKT()
	if err != nil {
		return fmt.Errorf("COTP: read CC: %w", err)
	}
	return c.parseCC(buf)
}

// Accept reads a COTP Connection Request (CR) and sends a Connection Confirm (CC).
// Called by a server after accepting a TCP connection.
func (c *Conn) Accept() error {
	buf, err := c.readTPKT()
	if err != nil {
		return fmt.Errorf("COTP: read CR: %w", err)
	}
	if err := c.parseCR(buf); err != nil {
		return fmt.Errorf("COTP: parse CR: %w", err)
	}
	cc := c.buildCC()
	if err := c.sendRaw(cc); err != nil {
		return fmt.Errorf("COTP: send CC: %w", err)
	}
	return nil
}

// Send sends payload as one or more COTP DT PDUs, segmenting if the payload
// exceeds the negotiated TPDU size. The receiver reassembles segments by reading
// DT PDUs until EOT=1 (see Receive).
func (c *Conn) Send(payload []byte) error {
	// DT header is 3 bytes (LI + PDU-type + TPDU-NR/EOT).
	// Max user data per TPDU = (1 << TPDUSizeCode) - 3.
	maxChunk := (1 << int(c.opts.TPDUSizeCode)) - 3
	for {
		if len(payload) <= maxChunk {
			return c.sendRaw(buildDTSegment(payload, true))
		}
		if err := c.sendRaw(buildDTSegment(payload[:maxChunk], false)); err != nil {
			return err
		}
		payload = payload[maxChunk:]
	}
}

// Receive reads one complete COTP TSDU and returns its payload.
// A TSDU may span multiple DT segments; segments are concatenated until EOT=1.
func (c *Conn) Receive() ([]byte, error) {
	var tsdu []byte
	for {
		buf, err := c.readTPKT()
		if err != nil {
			return nil, err
		}
		payload, eot, err := parseDT(buf)
		if err != nil {
			return nil, err
		}
		tsdu = append(tsdu, payload...)
		if eot {
			return tsdu, nil
		}
	}
}

// Close closes the underlying connection.
func (c *Conn) Close() error {
	return c.conn.Close()
}

// RemoteAddr returns the remote network address.
func (c *Conn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

// SetReadDeadline sets a deadline on the underlying TCP read.
// Pass zero to clear the deadline.
func (c *Conn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

// SetWriteDeadline sets a deadline on the underlying TCP write.
// Pass zero to clear the deadline.
func (c *Conn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

// ---- internal helpers ----

// sendRaw writes a complete TPKT-wrapped COTP PDU.
func (c *Conn) sendRaw(cotpPDU []byte) error {
	totalLen := tpktHeaderSize + len(cotpPDU)
	buf := make([]byte, totalLen)
	buf[0] = tpktVersion
	buf[1] = tpktReserved
	binary.BigEndian.PutUint16(buf[2:], uint16(totalLen))
	copy(buf[tpktHeaderSize:], cotpPDU)
	_, err := c.conn.Write(buf)
	return err
}

// readTPKT reads exactly one TPKT packet and returns the COTP PDU bytes.
func (c *Conn) readTPKT() ([]byte, error) {
	header := make([]byte, tpktHeaderSize)
	if _, err := io.ReadFull(c.conn, header); err != nil {
		return nil, fmt.Errorf("COTP: read TPKT header: %w", err)
	}
	if header[0] != tpktVersion {
		return nil, fmt.Errorf("COTP: invalid TPKT version 0x%02X", header[0])
	}
	totalLen := int(binary.BigEndian.Uint16(header[2:]))
	if totalLen < tpktHeaderSize {
		return nil, fmt.Errorf("COTP: TPKT length %d too small", totalLen)
	}
	payloadLen := totalLen - tpktHeaderSize
	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(c.conn, payload); err != nil {
		return nil, fmt.Errorf("COTP: read TPKT payload: %w", err)
	}
	return payload, nil
}

// buildCR builds a Connection Request PDU.
func (c *Conn) buildCR() []byte {
	opts := buildOptions(c.opts)
	hdrLen := 6 + len(opts)
	buf := make([]byte, 1+hdrLen)
	pos := 0
	buf[pos] = byte(hdrLen) // LI: header length - 1
	pos++
	buf[pos] = pduCR
	pos++
	binary.BigEndian.PutUint16(buf[pos:], 0) // dst ref = 0 in CR
	pos += 2
	binary.BigEndian.PutUint16(buf[pos:], c.localRef)
	pos += 2
	buf[pos] = 0x00 // class 0
	pos++
	copy(buf[pos:], opts)
	return buf
}

// buildCC builds a Connection Confirm PDU.
func (c *Conn) buildCC() []byte {
	opts := buildOptions(c.opts)
	hdrLen := 6 + len(opts)
	buf := make([]byte, 1+hdrLen)
	pos := 0
	buf[pos] = byte(hdrLen)
	pos++
	buf[pos] = pduCC
	pos++
	binary.BigEndian.PutUint16(buf[pos:], c.remoteRef)
	pos += 2
	binary.BigEndian.PutUint16(buf[pos:], c.localRef)
	pos += 2
	buf[pos] = 0x00 // class 0
	pos++
	copy(buf[pos:], opts)
	return buf
}

// parseCR parses a Connection Request PDU and negotiates TPDU size.
// The negotiated size is min(our default, client's proposed), stored in c.opts.TPDUSizeCode.
func (c *Conn) parseCR(buf []byte) error {
	if len(buf) < 7 {
		return fmt.Errorf("COTP: CR too short (%d)", len(buf))
	}
	if buf[1] != pduCR {
		return fmt.Errorf("COTP: expected CR (0xE0), got 0x%02X", buf[1])
	}
	c.remoteRef = binary.BigEndian.Uint16(buf[4:6])

	// Parse variable-length options that follow the fixed 7-byte header.
	li := int(buf[0]) // header length including all bytes after LI itself
	i := 7            // first option byte (after LI + pduType + dstRef(2) + srcRef(2) + class(1))
	for i < li+1 && i+1 < len(buf) {
		code := buf[i]
		length := int(buf[i+1])
		if i+2+length > len(buf) {
			break
		}
		if code == optTPDUSize && length == 1 {
			proposed := buf[i+2]
			// Negotiate: take the smaller of the two sides.
			if proposed < c.opts.TPDUSizeCode {
				c.opts.TPDUSizeCode = proposed
			}
		}
		i += 2 + length
	}
	return nil
}

// parseCC parses a Connection Confirm PDU.
func (c *Conn) parseCC(buf []byte) error {
	if len(buf) < 7 {
		return fmt.Errorf("COTP: CC too short (%d)", len(buf))
	}
	if buf[1] != pduCC {
		return fmt.Errorf("COTP: expected CC (0xD0), got 0x%02X", buf[1])
	}
	return nil
}

// buildDTSegment builds one COTP DT PDU segment.
// eot=true sets the EOT bit (last segment); eot=false marks an intermediate segment.
func buildDTSegment(payload []byte, eot bool) []byte {
	buf := make([]byte, 3+len(payload))
	buf[0] = 0x02 // LI = 2 (two bytes follow: PDU-type + TPDU-NR/EOT)
	buf[1] = pduDT
	if eot {
		buf[2] = 0x80 // TPDU-NR=0, EOT=1
	} else {
		buf[2] = 0x00 // TPDU-NR=0, EOT=0
	}
	copy(buf[3:], payload)
	return buf
}

// parseDT parses a Data PDU and returns the payload and the EOT (end-of-TSDU) flag.
// EOT is bit 7 of the TPDU-NR byte (third header byte). When EOT=0, more segments follow.
func parseDT(buf []byte) (payload []byte, eot bool, err error) {
	if len(buf) < 3 {
		return nil, false, fmt.Errorf("COTP: DT too short (%d)", len(buf))
	}
	if buf[1] != pduDT {
		return nil, false, fmt.Errorf("COTP: expected DT (0xF0), got 0x%02X", buf[1])
	}
	li := int(buf[0])
	if li+1 > len(buf) {
		return nil, false, fmt.Errorf("COTP: DT header length %d exceeds buffer %d", li, len(buf))
	}
	eot = buf[2]&0x80 != 0
	return buf[li+1:], eot, nil
}

// buildOptions serializes the options portion of CR/CC.
func buildOptions(opts Options) []byte {
	var out []byte
	if opts.TPDUSizeCode != 0 {
		out = append(out, optTPDUSize, 0x01, opts.TPDUSizeCode)
	}
	if len(opts.SrcTSel.Value) > 0 {
		out = append(out, optSrcTSel)
		out = append(out, byte(len(opts.SrcTSel.Value)))
		out = append(out, opts.SrcTSel.Value...)
	}
	if len(opts.DstTSel.Value) > 0 {
		out = append(out, optDstTSel)
		out = append(out, byte(len(opts.DstTSel.Value)))
		out = append(out, opts.DstTSel.Value...)
	}
	return out
}

// Dial establishes a COTP connection to the given address.
func Dial(address string, opts Options) (*Conn, error) {
	tcpConn, err := net.Dial("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("COTP: TCP dial %s: %w", address, err)
	}
	c := NewConn(tcpConn, opts)
	if err := c.Connect(); err != nil {
		tcpConn.Close()
		return nil, err
	}
	return c, nil
}

// Listener is created by a TCP listener and returns COTP connections.
type Listener struct {
	l    net.Listener
	opts Options
}

// ListenTCP creates a COTP listener on the given TCP address.
func ListenTCP(address string, opts Options) (*Listener, error) {
	l, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("COTP: listen %s: %w", address, err)
	}
	return &Listener{l: l, opts: opts}, nil
}

// Accept waits for and returns the next COTP connection.
func (l *Listener) Accept() (*Conn, error) {
	tcpConn, err := l.l.Accept()
	if err != nil {
		return nil, fmt.Errorf("COTP: accept: %w", err)
	}
	c := NewConn(tcpConn, l.opts)
	if err := c.Accept(); err != nil {
		tcpConn.Close()
		return nil, err
	}
	return c, nil
}

// Close closes the listener.
func (l *Listener) Close() error {
	return l.l.Close()
}

// Addr returns the listener's network address.
func (l *Listener) Addr() net.Addr {
	return l.l.Addr()
}
