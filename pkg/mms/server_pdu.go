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
	DebugHex("[MMS] ConfirmedRequest recv", buf)
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
	DebugHex("[MMS] AARQ recv", buf)
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

// ParseGetNameListRequestContent parses the content bytes of a GetNameList request.
// The C library uses tag [2] for continueAfter (non-standard); we also accept [5] per ISO 9506-2.
func ParseGetNameListRequestContent(content []byte) (*GetNameListRequest, error) {
	req := &GetNameListRequest{ObjectClass: -1, ObjectScope: ObjectScopeType(ObjectScopeVMD)}
	pos := 0
	for pos < len(content) {
		tlv, newPos, err := asn1ber.ParseTLV(content, pos)
		if err != nil {
			return req, nil
		}
		pos = newPos
		if tlv.Class != asn1ber.ClassContext {
			continue
		}
		switch tlv.Tag {
		case 0: // objectClass: [0] CONSTRUCTED { [0] integer }
			inner, _, err := asn1ber.ParseTLV(tlv.Value, 0)
			if err == nil && inner.Class == asn1ber.ClassContext && inner.Tag == 0 {
				v, _ := asn1ber.DecodeInteger(inner.Value)
				req.ObjectClass = ObjectClassType(v)
			}
		case 1: // objectScope: [1] CONSTRUCTED { choice }
			innerPos := 0
			for innerPos < len(tlv.Value) {
				inner, newInner, err := asn1ber.ParseTLV(tlv.Value, innerPos)
				if err != nil {
					break
				}
				innerPos = newInner
				if inner.Class != asn1ber.ClassContext {
					continue
				}
				switch inner.Tag {
				case 0: // vmdSpecific NULL
					req.ObjectScope = ObjectScopeType(ObjectScopeVMD)
				case 1: // domainSpecific: Identifier (the domain name)
					req.ObjectScope = ObjectScopeType(ObjectScopeDomain)
					req.DomainID = string(inner.Value)
				case 2: // aaSpecific NULL
					req.ObjectScope = ObjectScopeType(ObjectScopeAssoc)
				}
			}
		case 2: // continueAfter — C library non-standard tag (libiec61850 uses [2], standard says [5])
			req.ContinueAfter = string(tlv.Value)
		case 5: // continueAfter — ISO 9506-2 standard tag
			req.ContinueAfter = string(tlv.Value)
		}
	}
	// Logged at higher level (server.go) with event name and conn context.
	return req, nil
}

// ParseReadRequestContent parses the content of a Read confirmed service request.
//
// Wire layout (ISO 9506-2):
//
//	[0] IMPLICIT BOOLEAN OPTIONAL  -- specificationWithResult (ignored per IEC 61850)
//	[1] IMPLICIT variableAccessSpecification
func ParseReadRequestContent(content []byte) ([]VariableSpecification, error) {
	pos := 0

	// Skip optional [0] IMPLICIT BOOLEAN specificationWithResult (tag 0x80).
	// IEC 61850 servers ignore this flag; the C library does the same.
	if len(content) > 0 && content[0] == 0x80 {
		tlv, next, err := asn1ber.ParseTLV(content, 0)
		if err == nil && tlv.Class == asn1ber.ClassContext && tlv.Tag == 0 {
			pos = next
		}
	}

	// Strip [1] EXPLICIT variableAccessSpec
	varAccessTLV, _, err := asn1ber.ParseTLV(content, pos)
	if err != nil {
		return nil, fmt.Errorf("mms: ReadRequest: parse variableAccessSpec: %w", err)
	}
	if varAccessTLV.Class != asn1ber.ClassContext || varAccessTLV.Tag != 1 {
		return nil, fmt.Errorf("mms: ReadRequest: expected [1] variableAccessSpec, got class=%d tag=%d",
			varAccessTLV.Class, varAccessTLV.Tag)
	}

	// Strip [0] listOfVariable
	listTLV, _, err2 := asn1ber.ParseTLV(varAccessTLV.Value, 0)
	if err2 != nil {
		return nil, fmt.Errorf("mms: ReadRequest: parse listOfVariable: %w", err2)
	}
	if listTLV.Class != asn1ber.ClassContext || listTLV.Tag != 0 {
		return nil, fmt.Errorf("mms: ReadRequest: expected [0] listOfVariable, got class=%d tag=%d",
			listTLV.Class, listTLV.Tag)
	}

	// Iterate 0x30 SEQUENCE items (ListOfVariableSeq entries)
	var specs []VariableSpecification
	pos = 0
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

// ParseGetVarAccessAttributesRequest parses the content bytes of a GetVariableAccessAttributes request.
// The request content is a VariableSpecification CHOICE, identical to what parseVariableSpec handles.
func ParseGetVarAccessAttributesRequest(content []byte) (VariableSpecification, error) {
	return parseVariableSpec(content)
}

// appContextNameMMS is the OID content bytes for the IEC 61850 MMS application context (1.0.9506.2.3).
var appContextNameMMS = []byte{0x28, 0xCA, 0x22, 0x02, 0x03}

// BuildAARE builds an ACSE AARE (Association Response) PDU wrapping the MMS initiate response.
// Uses the libiec61850-compatible EXTERNAL (0x28) wrapping for user-information.
func BuildAARE(mmsResponse []byte) []byte {
	Logf(RoleServer, EventInitiate, "sending AARE pduSize=%d", len(mmsResponse))
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

// BuildErrorResponse builds a ConfirmedErrorPDU matching the libiec61850 C wire format:
//
//	0xa2 len {
//	  0x80 len invokeId          // [0] invokeID (always present)
//	  0xa2 len {                 // [2] serviceError
//	    0xa0 3 {                 // [0] errorClass
//	      errClassTag 0x01 val   // e.g. 0x84=service, 0x87=access, 0x8c=others
//	    }
//	  }
//	}
func BuildErrorResponse(invokeID uint32, mmsErr Error) []byte {
	errClassTag, errVal := errorToClassTag(mmsErr)

	// errorClass: [0] CONSTRUCTED { classTag 0x01 val }
	errorClass := asn1ber.EncodeContextTLV(0, true, []byte{errClassTag, 0x01, errVal})

	// serviceError: [2] CONSTRUCTED { errorClass }
	serviceError := asn1ber.EncodeContextTLV(2, true, errorClass)

	// invokeID: [0] PRIMITIVE
	invokeIDBytes := asn1ber.EncodeContextTLV(0, false, asn1ber.EncodeIntegerContent(int64(invokeID)))

	body := append(invokeIDBytes, serviceError...)
	return appendTagLength(tagConfirmedError, body)
}

// errorToClassTag maps an mms.Error to the C library's (errorClassTag, value) pair.
func errorToClassTag(mmsErr Error) (tag byte, val byte) {
	switch mmsErr {
	case ErrAccessObjectAccessUnsupported:
		return 0x87, 1 // access[1] object-access-unsupported
	case ErrAccessObjectNonExistent:
		return 0x87, 2 // access[2] object-non-existent
	case ErrAccessObjectAccessDenied:
		return 0x87, 3 // access[3] object-access-denied
	case ErrDefinitionTypeUnsupported:
		return 0x82, 3 // definition[3] type-unsupported
	case ErrDefinitionObjectUndefined:
		return 0x82, 1 // definition[1] object-undefined
	default:
		return 0x84, 0 // service[0] other
	}
}

// BuildServiceErrorResponse builds a confirmed error PDU for a specific service.
func BuildServiceErrorResponse(invokeID uint32, mmsErr Error) []byte {
	return BuildErrorResponse(invokeID, mmsErr)
}

// BuildGetVarAccessAttributesResponse builds a GetVariableAccessAttributes response.
// typeSpecBytes is the BER-encoded TypeSpecification content (already built by the caller).
func BuildGetVarAccessAttributesResponse(invokeID uint32, typeSpecBytes []byte) []byte {
	var svcBody []byte
	// [0] IMPLICIT BOOLEAN mmsDeletable = false
	svcBody = append(svcBody, asn1ber.EncodeContextTLV(0, false, []byte{0x00})...)
	// [2] EXPLICIT TypeSpecification
	svcBody = append(svcBody, asn1ber.EncodeContextTLV(2, true, typeSpecBytes)...)
	// Service tag [6] = svcGetVariableAccessAttributes CONSTRUCTED
	svcTag := appendTagLength(byte(svcGetVariableAccessAttributes|asn1ber.ClassContext|asn1ber.Constructed), svcBody)
	return buildConfirmedResponse(invokeID, svcTag)
}

// --- TypeSpecification encoding helpers ---

// TypeSpecStructure encodes an MMS structure TypeSpecification with named components.
// Each component is a (name, typeSpec) pair; typeSpec is the already-encoded child TypeSpec.
func TypeSpecStructure(components []TypeSpecComponent) []byte {
	var compBody []byte
	for _, c := range components {
		// StructComponent is UNIVERSAL SEQUENCE { [0] name, [1] EXPLICIT typeSpec }
		var sc []byte
		sc = append(sc, asn1ber.EncodeContextTLV(0, false, []byte(c.Name))...)
		sc = append(sc, asn1ber.EncodeContextTLV(1, true, c.TypeSpec)...)
		compBody = append(compBody, asn1ber.EncodeTLV(asn1ber.ClassUniversal, true, asn1ber.TagSequence, sc)...)
	}
	// [1] IMPLICIT SEQUENCE OF components
	comps := asn1ber.EncodeContextTLV(1, true, compBody)
	// [2] IMPLICIT structure
	return asn1ber.EncodeContextTLV(2, true, comps)
}

// TypeSpecComponent is one named member of a structure TypeSpecification.
type TypeSpecComponent struct {
	Name     string
	TypeSpec []byte
}

// TypeSpecBoolean encodes an MMS boolean TypeSpecification.
func TypeSpecBoolean() []byte { return asn1ber.EncodeContextTLV(3, false, nil) }

// TypeSpecBitString encodes an MMS bit-string TypeSpecification (negative = fixed size).
func TypeSpecBitString(bits int) []byte {
	return asn1ber.EncodeContextTLV(4, false, asn1ber.EncodeIntegerContent(int64(bits)))
}

// TypeSpecFloat32 encodes an MMS 32-bit floating-point TypeSpecification.
func TypeSpecFloat32() []byte {
	// [7] CONSTRUCTED SEQUENCE { INTEGER formatwidth=32, INTEGER exponentwidth=8 }
	var body []byte
	body = append(body, asn1ber.EncodeInteger(32)...)
	body = append(body, asn1ber.EncodeInteger(8)...)
	return asn1ber.EncodeContextTLV(7, true, body)
}

// TypeSpecFloat64 encodes an MMS 64-bit floating-point TypeSpecification.
func TypeSpecFloat64() []byte {
	var body []byte
	body = append(body, asn1ber.EncodeInteger(64)...)
	body = append(body, asn1ber.EncodeInteger(11)...)
	return asn1ber.EncodeContextTLV(7, true, body)
}

// TypeSpecInteger encodes an MMS signed integer TypeSpecification (size in bytes).
func TypeSpecInteger(bytes int) []byte {
	return asn1ber.EncodeContextTLV(5, false, asn1ber.EncodeIntegerContent(int64(bytes)))
}

// TypeSpecUnsigned encodes an MMS unsigned integer TypeSpecification (size in bytes).
func TypeSpecUnsigned(bytes int) []byte {
	return asn1ber.EncodeContextTLV(6, false, asn1ber.EncodeIntegerContent(int64(bytes)))
}

// TypeSpecVisibleString encodes an MMS visible-string TypeSpecification (max length).
func TypeSpecVisibleString(maxLen int) []byte {
	return asn1ber.EncodeContextTLV(10, false, asn1ber.EncodeIntegerContent(int64(maxLen)))
}

// TypeSpecOctetString encodes an MMS octet-string TypeSpecification (max bytes).
func TypeSpecOctetString(maxBytes int) []byte {
	return asn1ber.EncodeContextTLV(9, false, asn1ber.EncodeIntegerContent(int64(maxBytes)))
}

// TypeSpecUTCTime encodes an MMS UTC-time TypeSpecification.
func TypeSpecUTCTime() []byte { return asn1ber.EncodeContextTLV(17, false, nil) }

// TypeSpecArray encodes an MMS array TypeSpecification.
// count is the number of elements; elementTypeSpec is the already-encoded element TypeSpec.
// Per ASN.1: array [1] CONSTRUCTED { numberOfElements [1] INTEGER, elementType [2] EXPLICIT TypeSpecification }
func TypeSpecArray(count int, elementTypeSpec []byte) []byte {
	var body []byte
	body = append(body, asn1ber.EncodeContextTLV(1, false, asn1ber.EncodeIntegerContent(int64(count)))...)
	body = append(body, asn1ber.EncodeContextTLV(2, true, elementTypeSpec)...)
	return asn1ber.EncodeContextTLV(1, true, body)
}

// TypeSpecBinaryTime encodes an MMS binary-time TypeSpecification.
// size must be 4 or 6 (bytes). Per ASN.1: binarytime BOOLEAN (true=6-byte, false=4-byte).
func TypeSpecBinaryTime(size int) []byte {
	v := byte(0x00)
	if size == 6 {
		v = 0x01
	}
	return asn1ber.EncodeContextTLV(12, false, []byte{v})
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
	// [0] listOfIdentifier: SEQUENCE OF Identifier (VisibleString per ISO 9506-2)
	var identifiers []byte
	for _, name := range names {
		identifiers = append(identifiers, asn1ber.EncodeVisibleString(name)...)
	}
	var listBody []byte
	listBody = append(listBody, asn1ber.EncodeContextTLV(0, true, identifiers)...)
	// moreFollows has DEFAULT TRUE per ISO 9506-2, so FALSE must be encoded explicitly —
	// an absent field means TRUE, causing clients to loop forever requesting more pages.
	if moreFollows {
		listBody = append(listBody, asn1ber.EncodeContextTLV(1, false, []byte{0xFF})...)
	} else {
		listBody = append(listBody, asn1ber.EncodeContextTLV(1, false, []byte{0x00})...)
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

// ParseGetNamedVarListAttrRequest parses the content of a GetNamedVariableListAttributes request.
// Returns the domain ID and item name (e.g. "Device1", "LLN0$dataset1").
func ParseGetNamedVarListAttrRequest(content []byte) (domainID, itemID string, err error) {
	tlv, _, e := asn1ber.ParseTLV(content, 0)
	if e != nil {
		return "", "", e
	}
	// Expect [1] CONSTRUCTED domainSpecific
	if tlv.Class != asn1ber.ClassContext || tlv.Tag != 1 {
		return "", "", fmt.Errorf("mms: GVLAA: expected [1] domainSpecific, got class=%d tag=%d", tlv.Class, tlv.Tag)
	}
	pos := 0
	for pos < len(tlv.Value) {
		inner, newPos, e := asn1ber.ParseTLV(tlv.Value, pos)
		if e != nil {
			break
		}
		pos = newPos
		if inner.Class == asn1ber.ClassUniversal && inner.Tag == asn1ber.TagVisibleString {
			if domainID == "" {
				domainID = string(inner.Value)
			} else {
				itemID = string(inner.Value)
			}
		}
	}
	return domainID, itemID, nil
}

// BuildGetNamedVarListAttrResponse builds a GetNamedVariableListAttributes response.
// deletable indicates if the list may be deleted by clients.
// members is the list of variable specifications that make up the named variable list.
func BuildGetNamedVarListAttrResponse(invokeID uint32, deletable bool, members []VariableSpecification) []byte {
	var svcBody []byte

	// mmsDeletable: [0] IMPLICIT BOOLEAN (tag 0x80, per GetNamedVariableListAttributesResponse.c:123-131)
	delByte := byte(0x00)
	if deletable {
		delByte = 0xff
	}
	svcBody = append(svcBody, asn1ber.EncodeContextTLV(0, false, []byte{delByte})...)

	// listOfVariable: [1] IMPLICIT SEQUENCE OF (tag 0xA1, per GetNamedVariableListAttributesResponse.c:88-89)
	// Each member is a SEQUENCE { variableSpecification VariableSpecification }
	var listBody []byte
	for _, m := range members {
		// domainSpecific: [1] CONSTRUCTED { 0x1a domainId, 0x1a itemId }
		var ds []byte
		ds = append(ds, 0x1a)
		ds = append(ds, asn1ber.EncodeLength(len(m.DomainID))...)
		ds = append(ds, []byte(m.DomainID)...)
		ds = append(ds, 0x1a)
		ds = append(ds, asn1ber.EncodeLength(len(m.ItemID))...)
		ds = append(ds, []byte(m.ItemID)...)
		// ObjectName.domainSpecific: [1] CONSTRUCTED
		objName := asn1ber.EncodeContextTLV(1, true, ds)
		// variableSpecification.name: [0] CONSTRUCTED
		varSpec := asn1ber.EncodeContextTLV(0, true, objName)
		// member SEQUENCE (universal 0x30)
		member := asn1ber.EncodeTLV(asn1ber.ClassUniversal, true, asn1ber.TagSequence, varSpec)
		listBody = append(listBody, member...)
	}
	svcBody = append(svcBody, asn1ber.EncodeContextTLV(1, true, listBody)...)

	// [12] CONSTRUCTED service tag
	svcTag := appendTagLength(byte(svcGetNamedVariableListAttributes|asn1ber.ClassContext|asn1ber.Constructed), svcBody)
	return buildConfirmedResponse(invokeID, svcTag)
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
