/*
 *  presentation.go
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

// Package isopresentation implements the ISO 8823 Presentation Protocol subset
// used by the IEC 61850 / MMS protocol stack.
//
// IEC 61850 uses a small subset of ISO 8823 (Normal Mode only):
//   - CP (Connect Presentation) PPDU: sent by client during association establishment.
//   - CPA (Connect Presentation Accept) PPDU: sent by server to accept.
//   - User-Data PDU: wraps MMS PDUs during normal data exchange.
//
// The Presentation layer maps application-context identifiers to abstract syntaxes:
//   - Context ID 1  → ACSE  (OID 2.2.1.0.1, i.e. 0x52 0x01 0x00 0x01)
//   - Context ID 3  → MMS   (OID 1.0.9506.2.1, i.e. 0x28 0xCA 0x22 0x02 0x01)
//
// Transfer syntax for both is BER (OID 2.1.1, i.e. 0x51 0x01).
package isopresentation

import "fmt"

// Context identifiers defined by the IEC 61850 presentation profile.
const (
	ContextIDACE = 1 // ACSE
	ContextIDMMS = 3 // MMS
)

// Fixed OID content bytes (without tag/length) for the presentation-context-definition-list.
var (
	oidACE = []byte{0x52, 0x01, 0x00, 0x01}       // 2.2.1.0.1
	oidMMS = []byte{0x28, 0xCA, 0x22, 0x02, 0x01} // 1.0.9506.2.1
	oidBER = []byte{0x51, 0x01}                   // 2.1.1 (BER transfer syntax)

	// callingPresentationSelector / calledPresentationSelector default values.
	defaultSelector = []byte{0x00, 0x00, 0x00, 0x01}
)

// BuildConnectAcceptPDU builds an ISO 8823 Connect Presentation Accept (CPA) PPDU
// for server-side use. acsePDU is the ACSE AARE to embed.
//
// Structure:
//
//	0x31 <len>
//	  0xa0 0x03 0x80 0x01 0x01             mode-selector = normal-mode(1)
//	  0xa2 <len>                            normal-mode-parameters
//	    0x83 0x04 0x00 0x00 0x00 0x01       responding-presentation-selector
//	    0xa5 0x12 [18 bytes]                context-definition-result-list
//	    0x61 <len>                          fully-encoded-data (ACSE AARE)
func BuildConnectAcceptPDU(acsePDU []byte) []byte {
	// context-definition-result-list: two acceptance items (one per context)
	// Each item: 0x30 0x07 0x80 0x01 0x00 0x81 0x02 0x51 0x01
	acceptItem := []byte{0x30, 0x07, 0x80, 0x01, 0x00, 0x81, 0x02, 0x51, 0x01}
	contextResults := []byte{0xa5, 0x12}
	contextResults = append(contextResults, acceptItem...) // for ACSE context
	contextResults = append(contextResults, acceptItem...) // for MMS context

	// fully-encoded-data with ACSE context ID
	fed := buildFullyEncodedData(acsePDU, ContextIDACE)

	// normal-mode-parameters content
	var nmp []byte
	// responding-presentation-selector [3] IMPLICIT OCTET STRING
	nmp = append(nmp, 0x83, byte(len(defaultSelector)))
	nmp = append(nmp, defaultSelector...)
	// context-definition-result-list
	nmp = append(nmp, contextResults...)
	// fully-encoded-data
	nmp = append(nmp, fed...)

	var cp []byte
	// mode-selector
	cp = append(cp, 0xa0, 0x03, 0x80, 0x01, 0x01)
	// normal-mode-parameters [2]
	cp = append(cp, 0xa2)
	cp = append(cp, encodeLength(len(nmp))...)
	cp = append(cp, nmp...)

	// Outer UNIVERSAL SET (0x31)
	out := []byte{0x31}
	out = append(out, encodeLength(len(cp))...)
	out = append(out, cp...)
	return out
}

// BuildConnectPDU builds an ISO 8823 Connect Presentation (CP) PPDU.
// acsePDU is the ACSE AARQ to embed as fully-encoded user data (context ID 1).
//
// Structure:
//
//	0x31 <len>   (UNIVERSAL SET)
//	  0xa0 0x03 0x80 0x01 0x01       mode-selector = normal-mode(1)
//	  0xa2 <len>                      normal-mode-parameters
//	    0x81 0x04 ...                 calling-presentation-selector
//	    0x82 0x04 ...                 called-presentation-selector
//	    0xa4 0x23 [35 bytes]          presentation-context-definition-list
//	    0x61 <len>                    fully-encoded-data
//	      0x30 <len>
//	        0x02 0x01 0x01            presentation-context-identifier = 1 (ACSE)
//	        0xa0 <len> [acsePDU]      single-ASN1-type encoding
func BuildConnectPDU(acsePDU []byte) []byte {
	// Build fully-encoded-data block
	fed := buildFullyEncodedData(acsePDU, ContextIDACE)

	// Build normal-mode-parameters content
	var nmp []byte
	// calling-presentation-selector [1] IMPLICIT OCTET STRING
	nmp = append(nmp, 0x81, byte(len(defaultSelector)))
	nmp = append(nmp, defaultSelector...)
	// called-presentation-selector [2] IMPLICIT OCTET STRING
	nmp = append(nmp, 0x82, byte(len(defaultSelector)))
	nmp = append(nmp, defaultSelector...)
	// presentation-context-definition-list [4]
	nmp = append(nmp, buildPCDL()...)
	// fully-encoded-data
	nmp = append(nmp, fed...)

	// Assemble CP PPDU
	var cp []byte
	// mode-selector [0] IMPLICIT SEQUENCE { mode-value [0] INTEGER := 1 }
	cp = append(cp, 0xa0, 0x03, 0x80, 0x01, 0x01)
	// normal-mode-parameters [2]
	cp = append(cp, 0xa2)
	cp = append(cp, encodeLength(len(nmp))...)
	cp = append(cp, nmp...)

	// Outer UNIVERSAL SET (0x31)
	out := []byte{0x31}
	out = append(out, encodeLength(len(cp))...)
	out = append(out, cp...)
	return out
}

// ParseConnectAcceptPDU parses an ISO 8823 CPA (Connect Presentation Accept) PPDU.
// It returns the ACSE AARE payload extracted from the fully-encoded-data section.
func ParseConnectAcceptPDU(buf []byte) ([]byte, error) {
	if len(buf) == 0 {
		return nil, fmt.Errorf("isopresentation: empty CPA PDU")
	}
	if buf[0] != 0x31 {
		return nil, fmt.Errorf("isopresentation: expected 0x31 (UNIVERSAL SET), got 0x%02X", buf[0])
	}
	length, offset, err := decodeLength(buf, 1)
	if err != nil {
		return nil, fmt.Errorf("isopresentation: decode CPA length: %w", err)
	}
	end := offset + length
	if end > len(buf) {
		return nil, fmt.Errorf("isopresentation: CPA buffer too short")
	}

	for offset < end {
		if offset >= end {
			break
		}
		tag := buf[offset]
		offset++
		tlen, newoff, err := decodeLength(buf, offset)
		if err != nil {
			return nil, fmt.Errorf("isopresentation: decode CPA element length: %w", err)
		}
		offset = newoff

		switch tag {
		case 0xa0: // mode-selector – skip
			offset += tlen
		case 0xa2: // normal-mode-parameters
			return parseNormalModeParameters(buf[offset : offset+tlen])
		default:
			offset += tlen
		}
	}
	return nil, fmt.Errorf("isopresentation: normal-mode-parameters not found in CPA")
}

// WrapUserData wraps an MMS PDU in a Presentation User-Data block.
// The MMS PDU is encoded under context ID 3 (MMS).
//
// Structure:
//
//	0x61 <len>
//	  0x30 <len>
//	    0x02 0x01 0x03    presentation-context-identifier = 3 (MMS)
//	    0xa0 <len> [mms]  single-ASN1-type encoding
func WrapUserData(mmsPDU []byte) []byte {
	return buildFullyEncodedData(mmsPDU, ContextIDMMS)
}

// UnwrapUserData extracts the payload from a Presentation User-Data block.
// It accepts blocks containing either the ACSE context (1) or MMS context (3).
func UnwrapUserData(buf []byte) ([]byte, error) {
	if len(buf) < 2 {
		return nil, fmt.Errorf("isopresentation: user-data PDU too short")
	}
	if buf[0] != 0x61 {
		return nil, fmt.Errorf("isopresentation: expected 0x61 (fully-encoded-data), got 0x%02X", buf[0])
	}
	length, offset, err := decodeLength(buf, 1)
	if err != nil {
		return nil, err
	}
	end := offset + length
	if end > len(buf) {
		return nil, fmt.Errorf("isopresentation: user-data buffer too short")
	}
	return parseFullyEncodedData(buf[offset:end])
}

// ---- internal helpers ----

// buildFullyEncodedData encodes the payload as a fully-encoded-data block
// with the given presentation context ID.
func buildFullyEncodedData(payload []byte, contextID int) []byte {
	// Inner: 0x02 0x01 <contextID> (3 bytes) + 0xa0 <len> <payload>
	inner := []byte{0x02, 0x01, byte(contextID)}
	inner = append(inner, 0xa0)
	inner = append(inner, encodeLength(len(payload))...)
	inner = append(inner, payload...)

	// PDV-list SEQUENCE
	var pdvList []byte
	pdvList = append(pdvList, 0x30)
	pdvList = append(pdvList, encodeLength(len(inner))...)
	pdvList = append(pdvList, inner...)

	// fully-encoded-data [APPLICATION 1] CONSTRUCTED
	var fed []byte
	fed = append(fed, 0x61)
	fed = append(fed, encodeLength(len(pdvList))...)
	fed = append(fed, pdvList...)
	return fed
}

// buildPCDL builds the fixed 37-byte presentation-context-definition-list
// (0xa4 tag + 35 bytes content defining contexts 1=ACSE and 3=MMS with BER syntax).
func buildPCDL() []byte {
	// ACSE context item: SEQUENCE { INTEGER 1, OID oidACE, SEQUENCE { OID oidBER } }
	acseItem := buildPCDLItem(ContextIDACE, oidACE)
	// MMS context item: SEQUENCE { INTEGER 3, OID oidMMS, SEQUENCE { OID oidBER } }
	mmsItem := buildPCDLItem(ContextIDMMS, oidMMS)

	content := append(acseItem, mmsItem...)

	var pcdl []byte
	pcdl = append(pcdl, 0xa4)
	pcdl = append(pcdl, encodeLength(len(content))...)
	pcdl = append(pcdl, content...)
	return pcdl
}

func buildPCDLItem(contextID int, abstractSyntaxOID []byte) []byte {
	// transfer-syntax-list: SEQUENCE { OID oidBER }
	berSyntax := []byte{0x06, byte(len(oidBER))}
	berSyntax = append(berSyntax, oidBER...)
	tsl := []byte{0x30, byte(len(berSyntax))}
	tsl = append(tsl, berSyntax...)

	// context-id INTEGER
	ctxID := []byte{0x02, 0x01, byte(contextID)}
	// abstract-syntax-name OID
	oid := []byte{0x06, byte(len(abstractSyntaxOID))}
	oid = append(oid, abstractSyntaxOID...)

	var content []byte
	content = append(content, ctxID...)
	content = append(content, oid...)
	content = append(content, tsl...)

	item := []byte{0x30, byte(len(content))}
	item = append(item, content...)
	return item
}

// parseNormalModeParameters parses the content of normal-mode-parameters [2]
// and returns the application payload from the fully-encoded-data element.
func parseNormalModeParameters(buf []byte) ([]byte, error) {
	offset := 0
	end := len(buf)
	for offset < end {
		if offset >= end {
			break
		}
		tag := buf[offset]
		offset++
		tlen, newoff, err := decodeLength(buf, offset)
		if err != nil {
			return nil, fmt.Errorf("isopresentation: decode nmp element length: %w", err)
		}
		offset = newoff

		switch tag {
		case 0x61: // fully-encoded-data
			return parseFullyEncodedData(buf[offset : offset+tlen])
		default:
			offset += tlen
		}
	}
	return nil, fmt.Errorf("isopresentation: fully-encoded-data (0x61) not found in normal-mode-parameters")
}

// parseFullyEncodedData extracts the application payload from a
// fully-encoded-data block (after the outer 0x61 tag has been consumed).
func parseFullyEncodedData(buf []byte) ([]byte, error) {
	offset := 0
	end := len(buf)
	if offset >= end || buf[offset] != 0x30 {
		return nil, fmt.Errorf("isopresentation: expected PDV-list SEQUENCE (0x30), got 0x%02X", safeGet(buf, offset))
	}
	offset++
	seqLen, newoff, err := decodeLength(buf, offset)
	if err != nil {
		return nil, fmt.Errorf("isopresentation: decode PDV-list length: %w", err)
	}
	offset = newoff
	seqEnd := offset + seqLen
	if seqEnd > end {
		seqEnd = end
	}

	for offset < seqEnd {
		tag := buf[offset]
		offset++
		tlen, newoff, err := decodeLength(buf, offset)
		if err != nil {
			return nil, err
		}
		offset = newoff

		switch tag {
		case 0x02: // presentation-context-identifier – skip
			offset += tlen
		case 0xa0: // single-ASN1-type encoding – this is the payload
			if offset+tlen > len(buf) {
				return nil, fmt.Errorf("isopresentation: payload exceeds buffer")
			}
			return buf[offset : offset+tlen], nil
		default:
			offset += tlen
		}
	}
	return nil, fmt.Errorf("isopresentation: payload (0xa0) not found in fully-encoded-data")
}

// encodeLength encodes a BER length field.
func encodeLength(length int) []byte {
	if length < 0x80 {
		return []byte{byte(length)}
	}
	if length <= 0xFF {
		return []byte{0x81, byte(length)}
	}
	return []byte{0x82, byte(length >> 8), byte(length)}
}

// decodeLength decodes a BER length starting at buf[offset].
// Returns (length, newOffset, error).
func decodeLength(buf []byte, offset int) (int, int, error) {
	if offset >= len(buf) {
		return 0, offset, fmt.Errorf("isopresentation: buffer too short for length at offset %d", offset)
	}
	b := buf[offset]
	offset++
	if b < 0x80 {
		return int(b), offset, nil
	}
	numBytes := int(b & 0x7F)
	if numBytes == 0 || numBytes > 3 {
		return 0, offset, fmt.Errorf("isopresentation: unsupported length encoding (0x%02X)", b)
	}
	if offset+numBytes > len(buf) {
		return 0, offset, fmt.Errorf("isopresentation: buffer too short for length bytes")
	}
	length := 0
	for i := 0; i < numBytes; i++ {
		length = (length << 8) | int(buf[offset])
		offset++
	}
	return length, offset, nil
}

func safeGet(buf []byte, offset int) byte {
	if offset < len(buf) {
		return buf[offset]
	}
	return 0
}
