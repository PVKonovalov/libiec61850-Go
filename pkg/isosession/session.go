/*
 *  session.go
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

// Package isosession implements the ISO 8327 Session Protocol subset used by
// the IEC 61850 / MMS protocol stack.
//
// IEC 61850 uses a very small subset of ISO 8327:
//   - Connect (CN) SPDU  (type 13): sent by client to establish a session.
//   - Accept (AC) SPDU   (type 14): sent by server to accept the session.
//   - Data Transfer SPDU (type  1): 4-byte header {0x01,0x00,0x01,0x00} + payload.
//   - Finish / Disconnect SPDUs for orderly close.
package isosession

import "fmt"

// SPDU type codes.
const (
	SPDUData       = 0x01 // Data Transfer (GT+DT concatenated)
	SPDUConnect    = 0x0D // Connect (CN)
	SPDUAccept     = 0x0E // Accept (AC)
	SPDUFinish     = 0x09 // Finish (FN)
	SPDUDisconnect = 0x0A // Disconnect (DN)
	SPDUAbort      = 0x19 // Abort
)

// dataSPDUHeader is the 4-byte header prepended to every user-data segment
// once a session is established. It represents GT(0x01,0x00) + DT(0x01,0x00).
var dataSPDUHeader = []byte{0x01, 0x00, 0x01, 0x00}

// BuildConnectSPDU builds an ISO 8327 Connect (CN) SPDU wrapping
// presentationPDU (the ISO Presentation CP PDU).
//
// CN SPDU layout:
//
//	[0x0D]  [LEN]  [22 parameter bytes]  [presentationPDU...]
//
// Parameter bytes:
//
//	PGI 0x05 len 6  - Connect/Accept Item
//	  PI 0x13 len 1  val 0x00  - Protocol Options
//	  PI 0x16 len 1  val 0x02  - Version Number = 2
//	PI  0x14 len 2  val 0x00 0x02  - Session Requirement (duplex)
//	PI  0x33 len 2  val 0x00 0x01  - Calling Session Selector
//	PI  0x34 len 2  val 0x00 0x01  - Called Session Selector
//	PGI 0xC1 len <N>               - User Data (payload)
func BuildConnectSPDU(presentationPDU []byte) []byte {
	plen := len(presentationPDU)

	// 22 parameter bytes (see layout above)
	params := [22]byte{
		// Connect-Accept-Item (PGI=0x05, len=6)
		0x05, 0x06,
		0x13, 0x01, 0x00, // Protocol Options = 0
		0x16, 0x01, 0x02, // Version Number = 2
		// Session Requirement (PI=0x14, len=2): duplex
		0x14, 0x02, 0x00, 0x02,
		// Calling Session Selector (PI=0x33, len=2): 0x0001
		0x33, 0x02, 0x00, 0x01,
		// Called Session Selector (PI=0x34, len=2): 0x0001
		0x34, 0x02, 0x00, 0x01,
		// User Data (PGI=0xC1, len=<payload length>)
		0xC1, byte(plen),
	}

	spdu := make([]byte, 0, 2+22+plen)
	spdu = append(spdu, SPDUConnect)   // type = 0x0D
	spdu = append(spdu, byte(22+plen)) // LEN = params (22) + payload
	spdu = append(spdu, params[:]...)
	spdu = append(spdu, presentationPDU...)
	return spdu
}

// ParseConnectResponseSPDU parses an Accept (AC) SPDU received from the server.
// It returns the embedded ISO Presentation payload (CPA PPDU).
func ParseConnectResponseSPDU(buf []byte) ([]byte, error) {
	if len(buf) < 2 {
		return nil, fmt.Errorf("isosession: SPDU too short (%d bytes)", len(buf))
	}
	if buf[0] != SPDUAccept {
		return nil, fmt.Errorf("isosession: expected ACCEPT SPDU (0x0E), got 0x%02X", buf[0])
	}
	length := int(buf[1])
	if len(buf) < 2+length {
		return nil, fmt.Errorf("isosession: SPDU buffer too short: need %d, have %d", 2+length, len(buf))
	}
	// Scan parameters looking for PGI 0xC1 (User Data)
	return extractUserData(buf[2 : 2+length])
}

// WrapDataSPDU wraps payload in a Data Transfer SPDU.
// The resulting bytes must be sent over COTP.
func WrapDataSPDU(payload []byte) []byte {
	data := make([]byte, 0, 4+len(payload))
	data = append(data, dataSPDUHeader...)
	data = append(data, payload...)
	return data
}

// UnwrapDataSPDU strips the 4-byte Data Transfer SPDU header and returns the payload.
func UnwrapDataSPDU(buf []byte) ([]byte, error) {
	if len(buf) < 4 {
		return nil, fmt.Errorf("isosession: data SPDU too short (%d bytes)", len(buf))
	}
	if buf[0] != 0x01 || buf[1] != 0x00 || buf[2] != 0x01 || buf[3] != 0x00 {
		return nil, fmt.Errorf("isosession: unexpected data SPDU header: %02X %02X %02X %02X",
			buf[0], buf[1], buf[2], buf[3])
	}
	return buf[4:], nil
}

// extractUserData scans an SPDU parameter block for PGI 0xC1 (User Data) and
// returns the bytes that follow it (which extend to the end of the SPDU body).
func extractUserData(params []byte) ([]byte, error) {
	i := 0
	for i < len(params) {
		if i+1 >= len(params) {
			break
		}
		pgi := params[i]
		plen := int(params[i+1])
		i += 2
		if pgi == 0xC1 {
			// User data starts immediately after this 2-byte header and runs
			// to the end of the buffer (plen is the payload length).
			if i+plen > len(params) {
				// Tolerate if the buffer is exactly right
				plen = len(params) - i
			}
			return params[i : i+plen], nil
		}
		i += plen
	}
	return nil, fmt.Errorf("isosession: PGI 0xC1 (user data) not found in SPDU parameters")
}

// BuildAcceptSPDU builds an ISO 8327 Accept (AC) SPDU for server-side use.
// presentationPDU is the ISO Presentation CPA PDU to embed.
//
// AC SPDU layout (18 parameter bytes, no Calling Session Selector):
//
//	[0x0E]  [LEN]  [18 parameter bytes]  [presentationPDU...]
func BuildAcceptSPDU(presentationPDU []byte) []byte {
	plen := len(presentationPDU)

	// 18 parameter bytes (no Calling Session Selector in AC vs CN)
	params := [18]byte{
		// Connect-Accept-Item (PGI=0x05, len=6)
		0x05, 0x06,
		0x13, 0x01, 0x00, // Protocol Options = 0
		0x16, 0x01, 0x02, // Version Number = 2
		// Session Requirement (PI=0x14, len=2): duplex
		0x14, 0x02, 0x00, 0x02,
		// Called Session Selector (PI=0x34, len=2): 0x0001
		0x34, 0x02, 0x00, 0x01,
		// User Data (PGI=0xC1, len=<payload length>)
		0xC1, byte(plen),
	}

	spdu := make([]byte, 0, 2+18+plen)
	spdu = append(spdu, SPDUAccept)    // type = 0x0E
	spdu = append(spdu, byte(18+plen)) // LEN
	spdu = append(spdu, params[:]...)
	spdu = append(spdu, presentationPDU...)
	return spdu
}

// ParseConnectSPDU parses a Connect (CN) SPDU received by a server.
// Returns the embedded ISO Presentation payload (CP PPDU).
func ParseConnectSPDU(buf []byte) ([]byte, error) {
	if len(buf) < 2 {
		return nil, fmt.Errorf("isosession: CN SPDU too short (%d bytes)", len(buf))
	}
	if buf[0] != SPDUConnect {
		return nil, fmt.Errorf("isosession: expected CONNECT SPDU (0x0D), got 0x%02X", buf[0])
	}
	length := int(buf[1])
	if len(buf) < 2+length {
		return nil, fmt.Errorf("isosession: CN SPDU buffer too short")
	}
	return extractUserData(buf[2 : 2+length])
}

// BuildFinishSPDU builds a Finish (FN) SPDU for orderly session release.
func BuildFinishSPDU(payload []byte) []byte {
	spdu := make([]byte, 0, 4+len(payload))
	spdu = append(spdu, SPDUFinish)
	spdu = append(spdu, byte(2+len(payload)))
	spdu = append(spdu, 0xC1, byte(len(payload))) // User Data PGI
	spdu = append(spdu, payload...)
	return spdu
}
