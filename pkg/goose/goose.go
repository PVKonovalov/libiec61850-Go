/*
 *  goose.go
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

// Package goose implements the IEC 61850 GOOSE (Generic Object Oriented Substation Event)
// protocol for publishing and subscribing to real-time events.
//
// GOOSE uses raw Ethernet frames (IEEE 802.3) with:
//   - EtherType: 0x88B8 (GOOSE)
//   - Optional VLAN tagging (IEEE 802.1Q, EtherType 0x8100)
//
// GOOSE frames are NOT transmitted over TCP/IP — they require raw socket access
// and are typically sent on a local network segment (link-local multicast).
//
// On Linux, raw sockets require root privileges or CAP_NET_RAW capability.
//
// Publisher example:
//
//	pub, err := goose.NewPublisher("eth0", goose.PublisherConfig{...})
//	pub.SetGooseCBRef("LD/LLN0$GO$gcbName")
//	pub.Publish(values)
//
// Subscriber example:
//
//	sub := goose.NewSubscriber("APP-ID")
//	sub.SetDataSetValues(handler)
//	recv := goose.NewReceiver("eth0")
//	recv.AddSubscriber(sub)
//	recv.Start()
package goose

import (
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"github.com/PVKonovalov/libiec61850-Go/pkg/asn1ber"
	"github.com/PVKonovalov/libiec61850-Go/pkg/iec61850/common"
	"github.com/PVKonovalov/libiec61850-Go/pkg/mms"
)

const (
	// EtherTypeGOOSE is the IEEE 802.3 EtherType for GOOSE frames.
	EtherTypeGOOSE = 0x88B8
	// EtherTypeVLAN is the EtherType for VLAN-tagged frames (802.1Q).
	EtherTypeVLAN = 0x8100

	// GoosePDU ASN.1 application tag
	goosePDUTag = 0x61 // [APPLICATION 1] IMPLICIT SEQUENCE

	// Retransmission schedule: after a state change GOOSE messages are sent
	// with exponentially increasing intervals up to maxRetransmitInterval.
	minRetransmitInterval = 4 * time.Millisecond
	maxRetransmitInterval = 1000 * time.Millisecond
)

// GoosePDU contains the decoded content of a GOOSE PDU.
// This mirrors the ASN.1 IECGoosePdu definition in IEC 61850-8-1.
type GoosePDU struct {
	GoCBRef           string       // GOOSE control block reference
	TimeAllowedToLive uint32       // milliseconds
	DataSet           string       // dataset reference
	GoID              string       // optional GOOSE identifier
	T                 mms.UTCTime  // event timestamp
	StNum             uint32       // state number (increments on state change)
	SqNum             uint32       // sequence number (increments each retransmission)
	Test              bool         // test mode flag
	ConfRev           uint32       // configuration revision
	NdsCom            bool         // needs commissioning flag
	NumDatSetEntries  uint32       // number of dataset entries
	AllData           []*mms.Value // dataset values
}

// EncodeGOOSEFrame encodes a complete GOOSE Ethernet frame.
// srcMAC is the sender's MAC address (6 bytes).
// comm holds the destination address and VLAN parameters.
// pdu is the GOOSE PDU to encode.
func EncodeGOOSEFrame(srcMAC [6]byte, comm common.PhyComAddress, pdu *GoosePDU) ([]byte, error) {
	pduBytes, err := EncodePDU(pdu)
	if err != nil {
		return nil, err
	}

	var frame []byte

	// Ethernet header: DST(6) + SRC(6) + [VLAN(4)] + EtherType(2)
	frame = append(frame, comm.DstAddress[:]...)
	frame = append(frame, srcMAC[:]...)

	if comm.VLANID != 0 || comm.VLANPriority != 0 {
		// 802.1Q VLAN tag
		frame = append(frame, 0x81, 0x00)
		tci := (uint16(comm.VLANPriority)<<13 | comm.VLANID)
		frame = append(frame, byte(tci>>8), byte(tci))
	}

	// EtherType GOOSE
	frame = append(frame, 0x88, 0xB8)

	// GOOSE header: AppID(2) + Length(2) + Reserved1(2) + Reserved2(2)
	frame = append(frame, byte(comm.AppID>>8), byte(comm.AppID))
	totalLen := 8 + len(pduBytes) // GOOSE header (8) + PDU
	frame = append(frame, byte(totalLen>>8), byte(totalLen))
	frame = append(frame, 0x00, 0x00) // Reserved1
	frame = append(frame, 0x00, 0x00) // Reserved2

	frame = append(frame, pduBytes...)
	return frame, nil
}

// EncodePDU encodes a GoosePDU into its BER representation.
func EncodePDU(pdu *GoosePDU) ([]byte, error) {
	var body []byte

	// [0] gocbRef
	body = append(body, encodeCtx(0, false, []byte(pdu.GoCBRef))...)
	// [1] timeAllowedToLive
	body = append(body, encodeCtx(1, false, encodeUint32Minimal(pdu.TimeAllowedToLive))...)
	// [2] datSet
	body = append(body, encodeCtx(2, false, []byte(pdu.DataSet))...)
	// [3] goID (optional)
	if pdu.GoID != "" {
		body = append(body, encodeCtx(3, false, []byte(pdu.GoID))...)
	}
	// [4] t (timestamp)
	tBytes := encodeUTCTime(pdu.T)
	body = append(body, encodeCtx(4, false, tBytes)...)
	// [5] stNum
	body = append(body, encodeCtx(5, false, encodeUint32Minimal(pdu.StNum))...)
	// [6] sqNum
	body = append(body, encodeCtx(6, false, encodeUint32Minimal(pdu.SqNum))...)
	// [7] test
	testByte := byte(0x00)
	if pdu.Test {
		testByte = 0xFF
	}
	body = append(body, encodeCtx(7, false, []byte{testByte})...)
	// [8] confRev
	body = append(body, encodeCtx(8, false, encodeUint32Minimal(pdu.ConfRev))...)
	// [9] ndsCom
	ndsComByte := byte(0x00)
	if pdu.NdsCom {
		ndsComByte = 0xFF
	}
	body = append(body, encodeCtx(9, false, []byte{ndsComByte})...)
	// [10] numDatSetEntries
	body = append(body, encodeCtx(10, false, encodeUint32Minimal(pdu.NumDatSetEntries))...)
	// [11] allData: SEQUENCE OF Data
	var allDataBody []byte
	for _, v := range pdu.AllData {
		enc, err := mms.EncodeValue(v)
		if err != nil {
			return nil, err
		}
		allDataBody = append(allDataBody, enc...)
	}
	body = append(body, encodeCtx(11, true, allDataBody)...)

	// Wrap in [APPLICATION 1] SEQUENCE
	return encodeAppTag(1, true, body), nil
}

// DecodeGOOSEFrame parses a GOOSE Ethernet frame and returns the PDU.
// buf should start with the GOOSE header (after Ethernet + optional VLAN headers).
func DecodeGOOSEFrame(buf []byte) (appID uint16, pdu *GoosePDU, err error) {
	if len(buf) < 8 {
		return 0, nil, fmt.Errorf("GOOSE: frame too short (%d)", len(buf))
	}
	appID = binary.BigEndian.Uint16(buf[0:2])
	totalLen := int(binary.BigEndian.Uint16(buf[2:4]))
	if totalLen < 8 || totalLen > len(buf) {
		return 0, nil, fmt.Errorf("GOOSE: invalid length %d", totalLen)
	}
	pduBytes := buf[8:totalLen]
	pdu, err = DecodePDU(pduBytes)
	return
}

// DecodePDU decodes a GOOSE PDU from BER bytes.
func DecodePDU(buf []byte) (*GoosePDU, error) {
	if len(buf) < 2 {
		return nil, fmt.Errorf("GOOSE: PDU too short")
	}
	// Expect [APPLICATION 1] CONSTRUCTED
	if buf[0] != goosePDUTag {
		return nil, fmt.Errorf("GOOSE: expected PDU tag 0x61, got 0x%02X", buf[0])
	}
	length, offset, err := asn1ber.DecodeLength(buf, 1)
	if err != nil {
		return nil, err
	}
	if offset+length > len(buf) {
		return nil, fmt.Errorf("GOOSE: PDU truncated")
	}
	body := buf[offset : offset+length]

	pdu := &GoosePDU{}
	pos := 0
	for pos < len(body) {
		tlv, newPos, err := asn1ber.ParseTLV(body, pos)
		if err != nil {
			return nil, err
		}
		pos = newPos

		if tlv.Class != asn1ber.ClassContext {
			continue
		}
		switch tlv.Tag {
		case 0: // gocbRef
			pdu.GoCBRef = string(tlv.Value)
		case 1: // timeAllowedToLive
			pdu.TimeAllowedToLive = decodeUint32(tlv.Value)
		case 2: // datSet
			pdu.DataSet = string(tlv.Value)
		case 3: // goID
			pdu.GoID = string(tlv.Value)
		case 4: // t
			pdu.T, _ = decodeUTCTime(tlv.Value)
		case 5: // stNum
			pdu.StNum = decodeUint32(tlv.Value)
		case 6: // sqNum
			pdu.SqNum = decodeUint32(tlv.Value)
		case 7: // test
			pdu.Test = len(tlv.Value) > 0 && tlv.Value[0] != 0
		case 8: // confRev
			pdu.ConfRev = decodeUint32(tlv.Value)
		case 9: // ndsCom
			pdu.NdsCom = len(tlv.Value) > 0 && tlv.Value[0] != 0
		case 10: // numDatSetEntries
			pdu.NumDatSetEntries = decodeUint32(tlv.Value)
		case 11: // allData
			elements, err := decodeAllData(tlv.Value)
			if err != nil {
				return nil, err
			}
			pdu.AllData = elements
		}
	}
	return pdu, nil
}

// ---- Publisher ----

// PublisherConfig holds the configuration for a GOOSE publisher.
type PublisherConfig struct {
	CommParams common.PhyComAddress
	Interface  string // Ethernet interface name (e.g., "eth0")
}

// Publisher publishes GOOSE messages on an Ethernet interface.
// It handles automatic retransmission scheduling per IEC 61850-8-1.
type Publisher struct {
	mu      sync.Mutex
	config  PublisherConfig
	pdu     GoosePDU
	srcMAC  [6]byte
	rawSock RawSocket // platform-specific raw socket
	running bool
	stopCh  chan struct{}
}

// RawSocket is the interface for sending raw Ethernet frames.
// Implementations are platform-specific (see rawsock_linux.go etc.).
type RawSocket interface {
	Send(frame []byte) error
	Close() error
}

// NewPublisher creates a new GOOSE publisher.
// interfaceName is the Ethernet interface (e.g., "eth0").
func NewPublisher(config PublisherConfig) (*Publisher, error) {
	sock, err := newRawSocket(config.Interface, EtherTypeGOOSE)
	if err != nil {
		return nil, fmt.Errorf("GOOSE publisher: open raw socket on %s: %w", config.Interface, err)
	}
	srcMAC, err := getInterfaceMAC(config.Interface)
	if err != nil {
		sock.Close()
		return nil, fmt.Errorf("GOOSE publisher: get MAC for %s: %w", config.Interface, err)
	}
	return &Publisher{
		config:  config,
		srcMAC:  srcMAC,
		rawSock: sock,
		stopCh:  make(chan struct{}),
		pdu: GoosePDU{
			TimeAllowedToLive: 2000,
			ConfRev:           1,
		},
	}, nil
}

// SetGooseCBRef sets the GOOSE control block reference.
func (p *Publisher) SetGooseCBRef(ref string) {
	p.mu.Lock()
	p.pdu.GoCBRef = ref
	p.mu.Unlock()
}

// SetDataSetRef sets the dataset reference.
func (p *Publisher) SetDataSetRef(ref string) {
	p.mu.Lock()
	p.pdu.DataSet = ref
	p.mu.Unlock()
}

// SetGoID sets the GOOSE identifier.
func (p *Publisher) SetGoID(id string) {
	p.mu.Lock()
	p.pdu.GoID = id
	p.mu.Unlock()
}

// SetConfRev sets the configuration revision number.
func (p *Publisher) SetConfRev(rev uint32) {
	p.mu.Lock()
	p.pdu.ConfRev = rev
	p.mu.Unlock()
}

// SetTimeAllowedToLive sets the time-allowed-to-live in milliseconds.
func (p *Publisher) SetTimeAllowedToLive(tTL uint32) {
	p.mu.Lock()
	p.pdu.TimeAllowedToLive = tTL
	p.mu.Unlock()
}

// Publish sends a GOOSE message with the given dataset values.
// This increments the state number (StNum) and resets the retransmission counter.
// Returns the number of bytes sent, or -1 on error.
func (p *Publisher) Publish(values []*mms.Value) (int, error) {
	p.mu.Lock()
	p.pdu.StNum++
	p.pdu.SqNum = 0
	p.pdu.AllData = values
	p.pdu.NumDatSetEntries = uint32(len(values))
	p.pdu.T = mms.UTCTimeFromTime(time.Now())
	pdu := p.pdu
	p.mu.Unlock()

	frame, err := EncodeGOOSEFrame(p.srcMAC, p.config.CommParams, &pdu)
	if err != nil {
		return -1, err
	}
	if err := p.rawSock.Send(frame); err != nil {
		return -1, err
	}
	return len(frame), nil
}

// Retransmit sends the current state again (same StNum, incremented SqNum).
// Used for the retransmission schedule.
func (p *Publisher) Retransmit() error {
	p.mu.Lock()
	p.pdu.SqNum++
	pdu := p.pdu
	p.mu.Unlock()

	frame, err := EncodeGOOSEFrame(p.srcMAC, p.config.CommParams, &pdu)
	if err != nil {
		return err
	}
	return p.rawSock.Send(frame)
}

// Close releases resources held by the publisher.
func (p *Publisher) Close() {
	p.mu.Lock()
	p.running = false
	p.mu.Unlock()
	p.rawSock.Close()
}

// ---- Subscriber ----

// Listener is called when a GOOSE message is received from a subscribed source.
// appID is the GOOSE App ID; pdu contains the decoded content.
type Listener func(appID uint16, pdu *GoosePDU)

// Subscriber filters and processes GOOSE messages for a specific AppID or control block.
type Subscriber struct {
	mu       sync.Mutex
	appID    uint16 // 0 = accept all
	goCBRef  string // filter by control block ref; "" = no filter
	listener Listener
}

// NewSubscriber creates a subscriber that filters by App ID.
// Use appID=0 to receive all GOOSE messages.
func NewSubscriber(appID uint16, listener Listener) *Subscriber {
	return &Subscriber{appID: appID, listener: listener}
}

// SetGoCBRef sets the optional GOOSE control block reference filter.
func (s *Subscriber) SetGoCBRef(ref string) {
	s.mu.Lock()
	s.goCBRef = ref
	s.mu.Unlock()
}

// dispatch is called by the Receiver when a GOOSE frame is received.
func (s *Subscriber) dispatch(appID uint16, pdu *GoosePDU) {
	s.mu.Lock()
	filterAppID := s.appID
	filterRef := s.goCBRef
	listener := s.listener
	s.mu.Unlock()

	if filterAppID != 0 && filterAppID != appID {
		return
	}
	if filterRef != "" && filterRef != pdu.GoCBRef {
		return
	}
	if listener != nil {
		listener(appID, pdu)
	}
}

// Receiver listens on an Ethernet interface for GOOSE frames and dispatches
// them to registered subscribers.
type Receiver struct {
	mu          sync.Mutex
	ifaceName   string
	subscribers []*Subscriber
	rawSock     RawSocket
	running     bool
	stopCh      chan struct{}
}

// NewReceiver creates a GOOSE receiver on the given Ethernet interface.
func NewReceiver(ifaceName string) *Receiver {
	return &Receiver{
		ifaceName: ifaceName,
		stopCh:    make(chan struct{}),
	}
}

// AddSubscriber registers a subscriber to receive GOOSE messages.
func (r *Receiver) AddSubscriber(s *Subscriber) {
	r.mu.Lock()
	r.subscribers = append(r.subscribers, s)
	r.mu.Unlock()
}

// Start begins receiving GOOSE frames in the background.
func (r *Receiver) Start() error {
	sock, err := newRawSocket(r.ifaceName, EtherTypeGOOSE)
	if err != nil {
		return fmt.Errorf("GOOSE receiver: open socket on %s: %w", r.ifaceName, err)
	}
	r.mu.Lock()
	r.rawSock = sock
	r.running = true
	r.mu.Unlock()

	go r.receiveLoop(sock)
	return nil
}

// Stop stops the receiver.
func (r *Receiver) Stop() {
	r.mu.Lock()
	r.running = false
	r.mu.Unlock()
	close(r.stopCh)
	if r.rawSock != nil {
		r.rawSock.Close()
	}
}

// receiveLoop reads raw frames from the socket and dispatches them.
func (r *Receiver) receiveLoop(sock RawSocket) {
	// Implementation delegates to platform-specific rawReceive.
	rawRecvLoop(sock, func(frame []byte) {
		r.handleFrame(frame)
	})
}

// handleFrame parses a raw GOOSE frame and dispatches to subscribers.
func (r *Receiver) handleFrame(frame []byte) {
	if len(frame) < 14 {
		return
	}

	// Skip Ethernet header: DST(6) + SRC(6) + EtherType(2)
	offset := 12
	etherType := binary.BigEndian.Uint16(frame[offset:])
	offset += 2

	// Strip VLAN tag if present
	if etherType == EtherTypeVLAN {
		if offset+4 > len(frame) {
			return
		}
		offset += 2 // skip TCI
		etherType = binary.BigEndian.Uint16(frame[offset:])
		offset += 2
	}

	if etherType != EtherTypeGOOSE {
		return
	}

	appID, pdu, err := DecodeGOOSEFrame(frame[offset:])
	if err != nil {
		return
	}

	r.mu.Lock()
	subs := r.subscribers
	r.mu.Unlock()

	for _, sub := range subs {
		sub.dispatch(appID, pdu)
	}
}

// ---- encoding helpers ----

func encodeCtx(tag int, constructed bool, value []byte) []byte {
	b := byte(asn1ber.ClassContext | tag)
	if constructed {
		b |= asn1ber.Constructed
	}
	length := asn1ber.EncodeLength(len(value))
	out := make([]byte, 1+len(length)+len(value))
	out[0] = b
	copy(out[1:], length)
	copy(out[1+len(length):], value)
	return out
}

func encodeAppTag(tag int, constructed bool, value []byte) []byte {
	b := byte(asn1ber.ClassApplication | tag)
	if constructed {
		b |= asn1ber.Constructed
	}
	length := asn1ber.EncodeLength(len(value))
	out := make([]byte, 1+len(length)+len(value))
	out[0] = b
	copy(out[1:], length)
	copy(out[1+len(length):], value)
	return out
}

func encodeUint32Minimal(v uint32) []byte {
	if v == 0 {
		return []byte{0}
	}
	var out []byte
	for v > 0 {
		out = append([]byte{byte(v)}, out...)
		v >>= 8
	}
	if out[0]&0x80 != 0 {
		out = append([]byte{0x00}, out...)
	}
	return out
}

func decodeUint32(buf []byte) uint32 {
	var v uint32
	for _, b := range buf {
		v = (v << 8) | uint32(b)
	}
	return v
}

func encodeUTCTime(t mms.UTCTime) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint32(buf[0:], t.Seconds)
	buf[4] = byte(t.Fractions >> 16)
	buf[5] = byte(t.Fractions >> 8)
	buf[6] = byte(t.Fractions)
	var q byte
	if t.LeapSecondsKnown {
		q |= 0x80
	}
	if t.ClockFailure {
		q |= 0x40
	}
	if t.ClockNotSynchronized {
		q |= 0x20
	}
	buf[7] = q
	return buf
}

func decodeUTCTime(buf []byte) (mms.UTCTime, error) {
	if len(buf) < 8 {
		return mms.UTCTime{}, fmt.Errorf("GOOSE: UTC time too short")
	}
	return mms.UTCTime{
		Seconds:              binary.BigEndian.Uint32(buf[0:4]),
		Fractions:            uint32(buf[4])<<16 | uint32(buf[5])<<8 | uint32(buf[6]),
		LeapSecondsKnown:     (buf[7] & 0x80) != 0,
		ClockFailure:         (buf[7] & 0x40) != 0,
		ClockNotSynchronized: (buf[7] & 0x20) != 0,
	}, nil
}

func decodeAllData(buf []byte) ([]*mms.Value, error) {
	var values []*mms.Value
	offset := 0
	for offset < len(buf) {
		v, newOff, err := mms.DecodeValue(buf, offset)
		if err != nil {
			return nil, err
		}
		values = append(values, v)
		offset = newOff
	}
	return values, nil
}
