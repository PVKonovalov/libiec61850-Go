/*
 *  acse.go
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

// Package acse implements the Association Control Service Element (ACSE)
// protocol (ISO 8649/8650) as used by the MMS/IEC 61850 communication stack.
//
// ACSE manages the association (session) between two applications. The key
// services are:
//   - A-ASSOCIATE (AARQ/AARE): establish an application association
//   - A-RELEASE  (RLRQ/RLRE): orderly release
//   - A-ABORT    (ABRT):       abortive release
//
// This package produces and consumes raw ACSE PDU bytes. The ACSE PDUs are
// then embedded in an ISO Presentation CP/CPA PDU and wrapped in an ISO
// Session CN/AC SPDU before being sent over COTP/TCP – see packages
// isopresentation and isosession.
package acse

import (
	"fmt"

	"github.com/PVKonovalov/libiec61850-Go/pkg/asn1ber"
)

// Application context OID for MMS/IEC 61850 (ISO 1.0.9506.2.3).
// Encoded as 5 raw OID content bytes (without tag or length prefix).
var appContextNameMMS = []byte{0x28, 0xCA, 0x22, 0x02, 0x03}

// Password authentication mechanism OID bytes (ISO 2.2.3.1): 0x52 0x03 0x01
var authMechPassword = []byte{0x52, 0x03, 0x01}

// AuthMechanism selects the ACSE authentication method.
type AuthMechanism int

const (
	AuthNone     AuthMechanism = 0
	AuthPassword AuthMechanism = 1
	AuthTLS      AuthMechanism = 2
)

// AuthParameter holds authentication credentials.
type AuthParameter struct {
	Mechanism    AuthMechanism
	PasswordData []byte // for AuthPassword
}

// ConnectionParams holds parameters for establishing an ACSE association.
type ConnectionParams struct {
	Auth *AuthParameter // nil = no authentication
}

// BuildAARQ builds an ACSE Association Request (AARQ) PDU.
// mmsInitiate is the pre-encoded MMS Initiate-Request PDU to embed as user data.
//
// Structure follows the libiec61850 / IEC 61850-8-1 encoding:
//
//	[APPLICATION 0] CONSTRUCTED  (0x60)
//	  [1] application-context-name: OID 1.0.9506.2.3
//	  [30] user-information       (0xbe)
//	    EXTERNAL                  (0x28)
//	      indirect-reference INTEGER := 3  (MMS context)
//	      [0] single-ASN1-type  := mmsInitiate
func BuildAARQ(params ConnectionParams, mmsInitiate []byte) ([]byte, error) {
	var body []byte

	// [1] application-context-name (0xa1 0x07 0x06 0x05 <5-byte OID>)
	body = append(body, 0xa1, 0x07)
	body = append(body, 0x06, byte(len(appContextNameMMS)))
	body = append(body, appContextNameMMS...)

	// Authentication (optional)
	if params.Auth != nil && params.Auth.Mechanism == AuthPassword {
		// [10] sender-acse-requirements: authentication bit
		body = append(body, 0x8a, 0x02, 0x04, 0x80)
		// [11] mechanism-name
		body = append(body, 0x8b, byte(len(authMechPassword)))
		body = append(body, authMechPassword...)
		// [12] calling-authentication-value [0] IMPLICIT OCTET STRING
		pw := params.Auth.PasswordData
		body = append(body, 0xac)
		pwInner := append([]byte{0x80, byte(len(pw))}, pw...)
		body = append(body, asn1ber.EncodeLength(len(pwInner))...)
		body = append(body, pwInner...)
	}

	// [30] user-information (0xbe) containing EXTERNAL (0x28)
	userInfo := buildUserInfo(mmsInitiate)
	body = append(body, 0xbe)
	body = append(body, asn1ber.EncodeLength(len(userInfo))...)
	body = append(body, userInfo...)

	// AARQ is [APPLICATION 0] CONSTRUCTED = 0x60
	aarq := []byte{0x60}
	aarq = append(aarq, asn1ber.EncodeLength(len(body))...)
	aarq = append(aarq, body...)
	return aarq, nil
}

// ParseAARE parses an ACSE Association Response (AARE) PDU received from a server.
// Returns the embedded MMS user data or an error if the association was rejected.
func ParseAARE(buf []byte) ([]byte, error) {
	// AARE is [APPLICATION 1] CONSTRUCTED = 0x61
	if len(buf) < 2 || buf[0] != 0x61 {
		return nil, fmt.Errorf("ACSE: expected AARE (0x61), got 0x%02X", safeFirst(buf))
	}
	length, offset, err := decodeLength(buf, 1)
	if err != nil {
		return nil, fmt.Errorf("ACSE: decode AARE length: %w", err)
	}
	end := offset + length
	if end > len(buf) {
		return nil, fmt.Errorf("ACSE: AARE buffer too short")
	}

	var result uint32 = 99
	var userData []byte

	for offset < end {
		tag := buf[offset]
		offset++
		tlen, newoff, err := decodeLength(buf, offset)
		if err != nil {
			return nil, fmt.Errorf("ACSE: parse AARE element: %w", err)
		}
		offset = newoff

		switch tag {
		case 0xa2: // [2] result
			if tlen >= 3 && buf[offset] == 0x02 {
				v, _, _ := decodeLength(buf, offset+1)
				_ = v
				if offset+2 < len(buf) {
					result = uint32(buf[offset+2])
				}
			}
			offset += tlen
		case 0xbe: // [30] user-information
			userData, err = parseUserInfo(buf[offset : offset+tlen])
			if err != nil {
				return nil, fmt.Errorf("ACSE: parse user-information: %w", err)
			}
			offset += tlen
		default:
			offset += tlen
		}
	}

	if result != 0 {
		return nil, fmt.Errorf("ACSE: association rejected (result=%d)", result)
	}
	if userData == nil {
		return nil, fmt.Errorf("ACSE: no user data in AARE")
	}
	return userData, nil
}

// BuildAARE builds an ACSE Association Response (AARE) PDU accepting the request.
// mmsResponse is the MMS Initiate-Response PDU to embed.
func BuildAARE(mmsResponse []byte) []byte {
	var body []byte

	// [1] application-context-name
	body = append(body, 0xa1, 0x07)
	body = append(body, 0x06, byte(len(appContextNameMMS)))
	body = append(body, appContextNameMMS...)

	// [2] result = 0 (accepted)
	body = append(body, 0xa2, 0x03, 0x02, 0x01, 0x00)

	// [3] result-source-diagnostic: acse-service-user(0), accepted(0)
	body = append(body, 0xa3, 0x05, 0xa1, 0x03, 0x02, 0x01, 0x00)

	// [30] user-information
	userInfo := buildUserInfo(mmsResponse)
	body = append(body, 0xbe)
	body = append(body, asn1ber.EncodeLength(len(userInfo))...)
	body = append(body, userInfo...)

	// AARE = [APPLICATION 1] CONSTRUCTED = 0x61
	aare := []byte{0x61}
	aare = append(aare, asn1ber.EncodeLength(len(body))...)
	aare = append(aare, body...)
	return aare
}

// BuildABRT builds an ACSE Abort (ABRT) PDU.
func BuildABRT(abortSource int) []byte {
	body := []byte{0x80, 0x01, byte(abortSource)} // [0] abort-source
	abrt := []byte{0x64}                          // [APPLICATION 4]
	abrt = append(abrt, asn1ber.EncodeLength(len(body))...)
	abrt = append(abrt, body...)
	return abrt
}

// ---- internal helpers ----

// buildUserInfo builds the EXTERNAL (0x28) structure that wraps MMS data
// inside the ACSE user-information field [30].
//
//	0x28 <len>
//	  0x02 0x01 0x03   indirect-reference (INTEGER 3 = MMS context)
//	  0xa0 <len> [mms] single-ASN1-type
func buildUserInfo(mmsData []byte) []byte {
	// indirect-reference: INTEGER 3
	inner := []byte{0x02, 0x01, 0x03}
	// single-ASN1-type: [0] IMPLICIT
	inner = append(inner, 0xa0)
	inner = append(inner, asn1ber.EncodeLength(len(mmsData))...)
	inner = append(inner, mmsData...)

	// EXTERNAL tag = UNIVERSAL CONSTRUCTED 8 = 0x28
	ext := []byte{0x28}
	ext = append(ext, asn1ber.EncodeLength(len(inner))...)
	ext = append(ext, inner...)
	return ext
}

// parseUserInfo extracts MMS data from an EXTERNAL (0x28) user-information block.
func parseUserInfo(buf []byte) ([]byte, error) {
	if len(buf) == 0 {
		return nil, fmt.Errorf("ACSE: empty user-information")
	}
	offset := 0
	// Expect EXTERNAL (0x28) tag
	if buf[offset] != 0x28 {
		// Some implementations omit the EXTERNAL wrapper; try scanning directly.
		return scanForSingleASN1(buf)
	}
	offset++
	extLen, newoff, err := decodeLength(buf, offset)
	if err != nil {
		return nil, err
	}
	offset = newoff
	return scanForSingleASN1(buf[offset : offset+extLen])
}

// scanForSingleASN1 finds the [0] single-ASN1-type element (0xa0 tag) and returns its content.
func scanForSingleASN1(buf []byte) ([]byte, error) {
	offset := 0
	for offset < len(buf) {
		tag := buf[offset]
		offset++
		tlen, newoff, err := decodeLength(buf, offset)
		if err != nil {
			return nil, err
		}
		offset = newoff
		if tag == 0xa0 {
			return buf[offset : offset+tlen], nil
		}
		offset += tlen
	}
	return nil, fmt.Errorf("ACSE: single-ASN1-type (0xa0) not found in user-information")
}

func decodeLength(buf []byte, offset int) (int, int, error) {
	if offset >= len(buf) {
		return 0, offset, fmt.Errorf("ACSE: buffer too short for length")
	}
	b := buf[offset]
	offset++
	if b < 0x80 {
		return int(b), offset, nil
	}
	numBytes := int(b & 0x7F)
	if numBytes > 3 {
		return 0, offset, fmt.Errorf("ACSE: length too large")
	}
	if offset+numBytes > len(buf) {
		return 0, offset, fmt.Errorf("ACSE: buffer too short for length bytes")
	}
	length := 0
	for i := 0; i < numBytes; i++ {
		length = (length << 8) | int(buf[offset])
		offset++
	}
	return length, offset, nil
}

func safeFirst(buf []byte) byte {
	if len(buf) > 0 {
		return buf[0]
	}
	return 0
}
