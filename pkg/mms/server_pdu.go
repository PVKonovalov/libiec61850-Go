/*
 *  server_pdu.go
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

package mms

// This file contains server-side PDU builders and request parsers.
// The client side is in pdu.go.

import (
	"fmt"

	"github.com/PVKonovalov/libiec61850-Go/pkg/asn1ber"
)

// Service tags — also exported for server-side dispatch.
const (
	SvcStatus              = svcStatus
	SvcGetNameList         = svcGetNameList
	SvcIdentify            = svcIdentify
	SvcRead                = svcRead
	SvcWrite               = svcWrite
	SvcGetVarAccessAttr    = svcGetVariableAccessAttributes
	SvcDefineNamedVarList  = svcDefineNamedVariableList
	SvcGetNamedVarListAttr = svcGetNamedVariableListAttributes
	SvcDeleteNamedVarList  = svcDeleteNamedVariableList
	SvcFileOpen            = svcFileOpen
	SvcFileRead            = svcFileRead
	SvcFileClose           = svcFileClose
	SvcFileDir             = svcFileDirectory
)

// ParseConfirmedRequest parses a ConfirmedRequestPDU and returns the
// invokeID, service tag, and service-specific content.
func ParseConfirmedRequest(buf []byte) (invokeID uint32, svcTag int, svcContent []byte, err error) {
	if len(buf) < 2 || buf[0] != tagConfirmedRequest {
		return 0, 0, nil, fmt.Errorf("mms: expected ConfirmedRequestPDU, got 0x%02X", buf[0])
	}
	length, offset, e := asn1ber.DecodeLength(buf, 1)
	if e != nil {
		return 0, 0, nil, e
	}
	body := buf[offset : offset+length]

	// [0] invokeId
	tlv, pos, e := asn1ber.ParseTLV(body, 0)
	if e != nil {
		return 0, 0, nil, fmt.Errorf("mms: parse invokeId: %w", e)
	}
	iv, e := asn1ber.DecodeUint32(tlv.Value)
	if e != nil {
		return 0, 0, nil, e
	}
	invokeID = iv

	// [1] confirmedServiceRequest  (present but we skip it in simplified parsing)
	// [2] ConfirmedServiceRequest CHOICE
	if pos >= len(body) {
		return invokeID, 0, nil, nil
	}
	svcTLV, _, e := asn1ber.ParseTLV(body, pos)
	if e != nil {
		return invokeID, 0, nil, fmt.Errorf("mms: parse service request: %w", e)
	}
	return invokeID, svcTLV.Tag, svcTLV.Value, nil
}

// ParseAARQ parses an ACSE AARQ PDU received by the server.
// Returns the MMS user data extracted from the user-information field.
func ParseAARQ(buf []byte) ([]byte, error) {
	if len(buf) < 2 {
		return nil, fmt.Errorf("mms: AARQ too short")
	}
	// AARQ is [APPLICATION 0] CONSTRUCTED = 0x60
	if buf[0] != 0x60 {
		return nil, fmt.Errorf("mms: expected AARQ (0x60), got 0x%02X", buf[0])
	}
	length, offset, err := asn1ber.DecodeLength(buf, 1)
	if err != nil {
		return nil, err
	}
	body := buf[offset : offset+length]

	// Scan for [30] user-information (tag 0xbe)
	pos := 0
	for pos < len(body) {
		tlv, newPos, err := asn1ber.ParseTLV(body, pos)
		if err != nil {
			return nil, err
		}
		pos = newPos
		// [30] user-information = context tag 30 constructed
		if tlv.Class == asn1ber.ClassContext && tlv.Tag == 30 {
			return extractMmsFromUserInfo(tlv.Value)
		}
	}
	return nil, fmt.Errorf("mms: no user-information found in AARQ")
}

// extractMmsFromUserInfo extracts the MMS PDU from ACSE user-information content.
// Handles both the EXTERNAL (0x28) wrapper used by libiec61850 and direct
// [0] encoding without a wrapper.
func extractMmsFromUserInfo(buf []byte) ([]byte, error) {
	pos := 0
	for pos < len(buf) {
		tlv, newPos, err := asn1ber.ParseTLV(buf, pos)
		if err != nil {
			break
		}
		pos = newPos

		// [0] single-ASN1-type directly
		if tlv.Class == asn1ber.ClassContext && tlv.Tag == 0 {
			return tlv.Value, nil
		}
		// EXTERNAL wrapper (UNIVERSAL CONSTRUCTED 8 = 0x28)
		if tlv.Class == asn1ber.ClassUniversal && tlv.Constructed && tlv.Tag == 8 {
			// Recurse into the EXTERNAL content
			return extractMmsFromUserInfo(tlv.Value)
		}
	}
	return nil, fmt.Errorf("mms: single-ASN1-type (0xa0) not found in AARQ user-information")
}

// ParseReadRequestContent parses the content of a Read confirmed service request.
// Wire: [1] EXPLICIT variableAccessSpec { [0] listOfVariable { 0x30 items... } }
func ParseReadRequestContent(content []byte) ([]VariableSpecification, error) {
	// Strip [1] EXPLICIT variableAccessSpec
	varAccessTLV, _, err := asn1ber.ParseTLV(content, 0)
	if err != nil {
		return nil, fmt.Errorf("mms: ReadRequest: parse variableAccessSpec: %w", err)
	}
	if varAccessTLV.Class != asn1ber.ClassContext || varAccessTLV.Tag != 1 {
		return nil, fmt.Errorf("mms: ReadRequest: expected [1] variableAccessSpec, got class=%d tag=%d",
			varAccessTLV.Class, varAccessTLV.Tag)
	}

	// Strip [0] listOfVariable
	listTLV, _, err := asn1ber.ParseTLV(varAccessTLV.Value, 0)
	if err != nil {
		return nil, fmt.Errorf("mms: ReadRequest: parse listOfVariable: %w", err)
	}
	if listTLV.Class != asn1ber.ClassContext || listTLV.Tag != 0 {
		return nil, fmt.Errorf("mms: ReadRequest: expected [0] listOfVariable, got class=%d tag=%d",
			listTLV.Class, listTLV.Tag)
	}

	// Iterate 0x30 SEQUENCE items (ListOfVariableSeq entries)
	var specs []VariableSpecification
	pos := 0
	for pos < len(listTLV.Value) {
		seqTLV, newPos, err := asn1ber.ParseTLV(listTLV.Value, pos)
		if err != nil {
			break
		}
		pos = newPos
		spec, err := parseVariableSpec(seqTLV.Value)
		if err != nil {
			continue
		}
		specs = append(specs, spec)
	}
	return specs, nil
}

// ParseWriteRequestContent parses the content of a Write service request.
// Both listOfVariable and listOfData use tag [0]; position disambiguates (ISO 9506).
func ParseWriteRequestContent(content []byte) ([]VariableSpecification, []*Value, error) {
	var specs []VariableSpecification
	var values []*Value

	pos := 0
	count0 := 0
	for pos < len(content) {
		tlv, newPos, err := asn1ber.ParseTLV(content, pos)
		if err != nil {
			break
		}
		pos = newPos

		if tlv.Class == asn1ber.ClassContext && tlv.Tag == 0 {
			count0++
			if count0 == 1 {
				// First [0] = listOfVariable: iterate 0x30 SEQUENCE items
				innerPos := 0
				for innerPos < len(tlv.Value) {
					seqTLV, newInner, err := asn1ber.ParseTLV(tlv.Value, innerPos)
					if err != nil {
						break
					}
					innerPos = newInner
					spec, err := parseVariableSpec(seqTLV.Value)
					if err == nil {
						specs = append(specs, spec)
					}
				}
			} else {
				// Second [0] = listOfData: Data values with no per-element wrapper
				innerPos := 0
				for innerPos < len(tlv.Value) {
					v, newInner, err := DecodeValue(tlv.Value, innerPos)
					if err != nil {
						break
					}
					innerPos = newInner
					values = append(values, v)
				}
			}
		}
	}
	return specs, values, nil
}

// parseVariableSpec parses a VariableSpecification from the content of a 0x30 ListOfVariableSeq entry.
// Supports both the correct wire format ([0] EXPLICIT name { [1] { 0x1a domainId 0x1a itemId } })
// and legacy context-tag format ([0] vmdSpecific, [1] { [0] domainId [1] itemId }).
func parseVariableSpec(content []byte) (VariableSpecification, error) {
	spec := VariableSpecification{ArrayIndex: -1}

	tlv, _, err := asn1ber.ParseTLV(content, 0)
	if err != nil {
		return spec, err
	}

	// [0] EXPLICIT name wrapper — strip it and parse inner CHOICE
	if tlv.Class == asn1ber.ClassContext && tlv.Tag == 0 && tlv.Constructed {
		tlv, _, err = asn1ber.ParseTLV(tlv.Value, 0)
		if err != nil {
			return spec, err
		}
	}

	if tlv.Class == asn1ber.ClassContext {
		switch tlv.Tag {
		case 0: // vmdSpecific
			spec.ItemID = string(tlv.Value)
		case 1: // domainSpecific
			innerPos := 0
			for innerPos < len(tlv.Value) {
				inner, newInner, err := asn1ber.ParseTLV(tlv.Value, innerPos)
				if err != nil {
					break
				}
				innerPos = newInner
				switch {
				case inner.Class == asn1ber.ClassUniversal && inner.Tag == asn1ber.TagVisibleString:
					// 0x1a VisibleString (correct format)
					if spec.DomainID == "" {
						spec.DomainID = string(inner.Value)
					} else {
						spec.ItemID = string(inner.Value)
					}
				case inner.Class == asn1ber.ClassContext && inner.Tag == 0:
					spec.DomainID = string(inner.Value)
				case inner.Class == asn1ber.ClassContext && inner.Tag == 1:
					spec.ItemID = string(inner.Value)
				}
			}
		case 2: // aa-specific
			spec.ItemID = string(tlv.Value)
		}
	}
	return spec, nil
}

// appContextNameMMS is the OID content bytes for the IEC 61850 MMS application context (1.0.9506.2.3).
var appContextNameMMS = []byte{0x28, 0xCA, 0x22, 0x02, 0x03}

// BuildAARE builds an ACSE AARE (Association Response) PDU wrapping the MMS initiate response.
// Uses the libiec61850-compatible EXTERNAL (0x28) wrapping for user-information.
func BuildAARE(mmsResponse []byte) []byte {
	var body []byte

	// [1] application-context-name
	body = append(body, 0xa1, 0x07, 0x06, byte(len(appContextNameMMS)))
	body = append(body, appContextNameMMS...)

	// [2] result: accepted (0)
	body = append(body, 0xa2, 0x03, 0x02, 0x01, 0x00)

	// [3] result-source-diagnostic: acse-service-user(0), accepted(0)
	body = append(body, 0xa3, 0x05, 0xa1, 0x03, 0x02, 0x01, 0x00)

	// [30] user-information (0xbe) containing EXTERNAL (0x28)
	inner := []byte{0x02, 0x01, 0x03} // indirect-reference = 3
	inner = append(inner, 0xa0)
	inner = append(inner, asn1ber.EncodeLength(len(mmsResponse))...)
	inner = append(inner, mmsResponse...)
	ext := []byte{0x28}
	ext = append(ext, asn1ber.EncodeLength(len(inner))...)
	ext = append(ext, inner...)
	body = append(body, 0xbe)
	body = append(body, asn1ber.EncodeLength(len(ext))...)
	body = append(body, ext...)

	// AARE is [APPLICATION 1] CONSTRUCTED = 0x61
	return appendTagLength(0x61, body)
}

// ---- Response builders ----

// BuildErrorResponse builds a minimal MMS error response PDU.
func BuildErrorResponse(invokeID uint32, mmsErr Error) []byte {
	var body []byte
	if invokeID > 0 {
		body = append(body, asn1ber.EncodeContextTLV(0, false, asn1ber.EncodeIntegerContent(int64(invokeID)))...)
	}
	// [1] serviceError
	errContent := asn1ber.EncodeContextTLV(0, false, asn1ber.EncodeIntegerContent(int64(mmsErr)))
	body = append(body, asn1ber.EncodeContextTLV(1, true, errContent)...)

	return appendTagLength(tagConfirmedError, body)
}

// BuildServiceErrorResponse builds a confirmed error PDU for a specific service.
func BuildServiceErrorResponse(invokeID uint32, mmsErr Error) []byte {
	return BuildErrorResponse(invokeID, mmsErr)
}

// BuildReadResponse builds a ConfirmedResponsePDU for a Read service.
func BuildReadResponse(invokeID uint32, results []*ReadResult) []byte {
	var listBody []byte
	for _, result := range results {
		if result.IsError {
			// [1] IMPLICIT DataAccessError (primitive 0x81)
			errBytes := asn1ber.EncodeIntegerContent(int64(result.Error))
			listBody = append(listBody, asn1ber.EncodeContextTLV(1, false, errBytes)...)
		} else {
			// Success: Data encoded directly with its own tag (no [0] wrapper)
			enc, err := EncodeValue(result.Value)
			if err != nil {
				enc, _ = EncodeValue(NewDataAccessError(DataAccessErrorObjectAccessDenied))
			}
			listBody = append(listBody, enc...)
		}
	}

	// [1] IMPLICIT listOfAccessResults, then [4] Read service response
	listOfResults := asn1ber.EncodeContextTLV(1, true, listBody)
	svcBody := appendTagLength(byte(svcRead|asn1ber.ClassContext|asn1ber.Constructed), listOfResults)
	return buildConfirmedResponse(invokeID, svcBody)
}

// BuildWriteResponse builds a ConfirmedResponsePDU for a Write service.
func BuildWriteResponse(invokeID uint32, results []WriteResult) []byte {
	var listBody []byte
	for _, result := range results {
		if result.Success {
			// [0] success: NULL
			listBody = append(listBody, asn1ber.EncodeContextTLV(0, false, nil)...)
		} else {
			// [1] failure: DataAccessError
			errBytes := asn1ber.EncodeIntegerContent(int64(result.Error))
			listBody = append(listBody, asn1ber.EncodeContextTLV(1, false, errBytes)...)
		}
	}

	svcBody := appendTagLength(byte(svcWrite|asn1ber.ClassContext|asn1ber.Constructed), listBody)
	return buildConfirmedResponse(invokeID, svcBody)
}

// BuildGetNameListResponse builds a GetNameList response PDU.
func BuildGetNameListResponse(invokeID uint32, names []string, moreFollows bool) []byte {
	var listBody []byte
	for _, name := range names {
		listBody = append(listBody, asn1ber.EncodeContextTLV(0, false, []byte(name))...)
	}
	if moreFollows {
		listBody = append(listBody, asn1ber.EncodeContextTLV(1, false, []byte{0xFF})...)
	}

	svcBody := appendTagLength(byte(svcGetNameList|asn1ber.ClassContext|asn1ber.Constructed), listBody)
	return buildConfirmedResponse(invokeID, svcBody)
}

// BuildIdentifyResponse builds an Identify response PDU.
func BuildIdentifyResponse(invokeID uint32, vendorName, modelName, revision string) []byte {
	var body []byte
	body = append(body, asn1ber.EncodeContextTLV(0, false, []byte(vendorName))...)
	body = append(body, asn1ber.EncodeContextTLV(1, false, []byte(modelName))...)
	body = append(body, asn1ber.EncodeContextTLV(2, false, []byte(revision))...)

	svcBody := appendTagLength(byte(svcIdentify|asn1ber.ClassContext|asn1ber.Constructed), body)
	return buildConfirmedResponse(invokeID, svcBody)
}

// buildConfirmedResponse wraps service PDU in a ConfirmedResponsePDU.
// invokeId uses UNIVERSAL INTEGER (0x02) per ISO 9506 / ConfirmedResponsePDU ASN.1 definition.
func buildConfirmedResponse(invokeID uint32, svcPDU []byte) []byte {
	var body []byte
	body = append(body, asn1ber.EncodeUnsigned(invokeID)...)
	body = append(body, svcPDU...)
	return appendTagLength(tagConfirmedResponse, body)
}

// appendTagLength prepends a tag and length to a content slice.
func appendTagLength(tag byte, content []byte) []byte {
	length := asn1ber.EncodeLength(len(content))
	out := make([]byte, 1+len(length)+len(content))
	out[0] = tag
	copy(out[1:], length)
	copy(out[1+len(length):], content)
	return out
}
