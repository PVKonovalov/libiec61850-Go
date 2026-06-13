/*
 *  pdu.go
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

import (
	"fmt"

	"github.com/PVKonovalov/libiec61850-Go/pkg/asn1ber"
)

// MMS PDU type tags (ISO 9506-1, context tags within MmsPdu CHOICE)
const (
	tagConfirmedRequest    = 0xA0 // [0] ConfirmedRequestPDU
	tagConfirmedResponse   = 0xA1 // [1] ConfirmedResponsePDU
	tagConfirmedError      = 0xA2 // [2] ConfirmedErrorPDU
	tagUnconfirmedPDU      = 0xA3 // [3] UnconfirmedPDU
	tagRejectPDU           = 0xA4 // [4] RejectPDU
	tagCancelRequestPDU    = 0xA5 // [5] CancelRequestPDU
	tagCancelResponsePDU   = 0xA6 // [6] CancelResponsePDU
	tagCancelErrorPDU      = 0xA7 // [7] CancelErrorPDU
	tagInitiateRequestPDU  = 0xA8 // [8] InitiateRequestPDU
	tagInitiateResponsePDU = 0xA9 // [9] InitiateResponsePDU
	tagInitiateErrorPDU    = 0xAA // [10] InitiateErrorPDU
	tagConcludeRequestPDU  = 0xAB // [11] ConcludeRequestPDU
	tagConcludeResponsePDU = 0xAC // [12] ConcludeResponsePDU
	tagConcludeErrorPDU    = 0xAD // [13] ConcludeErrorPDU
)

// MMS confirmed service request/response tags
const (
	svcStatus                         = 0
	svcGetNameList                    = 1
	svcIdentify                       = 2
	svcRename                         = 3
	svcRead                           = 4
	svcWrite                          = 5
	svcGetVariableAccessAttributes    = 6
	svcDefineNamedVariable            = 7
	svcDefineScatteredAccess          = 8
	svcGetScatteredAccessAttributes   = 9
	svcDeleteVariableAccess           = 10
	svcDefineNamedVariableList        = 11
	svcGetNamedVariableListAttributes = 12
	svcDeleteNamedVariableList        = 13
	svcDefineNamedType                = 14
	svcGetNamedTypeAttributes         = 15
	svcInput                          = 16
	svcOutput                         = 17
	svcTakeControl                    = 18
	svcRelinquishControl              = 19
	svcDefineSemaphore                = 20
	svcDeleteSemaphore                = 21
	svcReportSemaphoreStatus          = 22
	svcReportPoolSemaphoreStatus      = 23
	svcReportSemaphoreEntryStatus     = 24
	svcInitiateDownloadSequence       = 25
	svcDownloadSegment                = 26
	svcTerminateDownloadSequence      = 27
	svcInitiateUploadSequence         = 28
	svcUploadSegment                  = 29
	svcTerminateUploadSequence        = 30
	svcRequestDomainDownload          = 31
	svcRequestDomainUpload            = 32
	svcLoadDomainContent              = 33
	svcStoreDomainContent             = 34
	svcDeleteDomain                   = 35
	svcGetDomainAttributes            = 36
	svcCreateProgramInvocation        = 37
	svcDeleteProgramInvocation        = 38
	svcStart                          = 39
	svcStop                           = 40
	svcResume                         = 41
	svcReset                          = 42
	svcKill                           = 43
	svcGetProgramInvocationAttributes = 44
	svcObtainFile                     = 45
	svcFileOpen                       = 72
	svcFileRead                       = 73
	svcFileClose                      = 74
	svcFileRename                     = 75
	svcFileDelete                     = 76
	svcFileDirectory                  = 77
)

// Unconfirmed service tags
const (
	svcUnconfirmedStatus                 = 0
	svcUnconfirmedInformationReport      = 1
	svcUnconfirmedUnsolicitedStatus      = 2
	svcUnconfirmedEventNotification      = 3
	svcUnconfirmedAttachToEventCondition = 4
	svcUnconfirmedAttachToSemaphore      = 5
)

// ObjectClass values (used in GetNameList)
const (
	ObjectClassNamedVariable     = 0
	ObjectClassScatteredAccess   = 1
	ObjectClassNamedVariableList = 2
	ObjectClassNamedType         = 3
	ObjectClassSemaphore         = 4
	ObjectClassEventCondition    = 5
	ObjectClassEventAction       = 6
	ObjectClassEventEnrollment   = 7
	ObjectClassJournal           = 8
	ObjectClassDomain            = 9
	ObjectClassProgramInvocation = 10
	ObjectClassOperatorStation   = 11
)

// ObjectScope values (domain-specific or vmd-specific)
const (
	ObjectScopeVMD    = 0 // VMD-specific
	ObjectScopeDomain = 1 // domain-specific
	ObjectScopeAssoc  = 2 // association-specific
)

// InitiateRequestPDU represents the MMS connection initiation request.
// It negotiates protocol parameters between client and server.
type InitiateRequestPDU struct {
	LocalDetailCalling                int32
	ProposedMaxServOutstanding        int16
	ProposedMaxServOutstandingCalling int16
	ProposedDataStructureNestingLevel int8
	// ServicesSupportedCalling is a bitstring of supported services
	ServicesSupportedCalling []byte
}

// InitiateResponsePDU represents the server's response to InitiateRequestPDU.
type InitiateResponsePDU struct {
	LocalDetailCalled                   int32
	NegotiatedMaxServOutstanding        int16
	NegotiatedMaxServOutstandingCalled  int16
	NegotiatedDataStructureNestingLevel int8
	ServicesSupportedCalled             []byte
}

// VariableSpecification identifies an MMS variable for read/write operations.
type VariableSpecification struct {
	DomainID      string // IEC 61850: logical device name
	ItemID        string // IEC 61850: logical node + data object path
	ArrayIndex    int    // -1 if not an array access
	ComponentName string // optional sub-component
}

// ReadRequest represents an MMS Read service request.
type ReadRequest struct {
	Variables []VariableSpecification
}

// ReadResponse represents an MMS Read service response.
type ReadResponse struct {
	Results []*ReadResult
}

// ReadResult holds either a value or an error for one variable in a read response.
type ReadResult struct {
	Value   *Value
	Error   DataAccessError
	IsError bool
}

// WriteRequest represents an MMS Write service request.
type WriteRequest struct {
	Variables []VariableSpecification
	Values    []*Value
}

// WriteResponse represents an MMS Write service response.
type WriteResponse struct {
	Results []WriteResult
}

// WriteResult is the outcome for one variable in a write response.
type WriteResult struct {
	Success bool
	Error   DataAccessError
}

// GetNameListRequest represents an MMS GetNameList request.
type GetNameListRequest struct {
	ObjectClass   ObjectClassType
	ObjectScope   ObjectScopeType
	DomainID      string
	ContinueAfter string
}

// ObjectClassType is the type of object being listed.
type ObjectClassType int

// ObjectScopeType is the scope of the name list operation.
type ObjectScopeType int

// GetNameListResponse is the response to a GetNameList request.
type GetNameListResponse struct {
	Names       []string
	MoreFollows bool
}

// InformationReport is an unconfirmed MMS report (used for GOOSE and reporting).
type InformationReport struct {
	VariableListName VariableSpecification
	Values           []*Value
}

// ---- PDU encoding ----

// EncodeInitiateRequest builds the wire-format MMS Initiate Request PDU.
func EncodeInitiateRequest() []byte {
	// Default MMS initiate parameters for IEC 61850
	var body []byte

	// [0] localDetailCalling: 65000
	detail := asn1ber.EncodeIntegerContent(65000)
	body = append(body, asn1ber.EncodeContextTLV(0, false, detail)...)

	// [1] proposedMaxServOutstandingCalling: 5
	body = append(body, asn1ber.EncodeContextTLV(1, false, asn1ber.EncodeIntegerContent(5))...)

	// [2] proposedMaxServOutstandingCalled: 5
	body = append(body, asn1ber.EncodeContextTLV(2, false, asn1ber.EncodeIntegerContent(5))...)

	// [4] proposedDataStructureNestingLevel: 10
	body = append(body, asn1ber.EncodeContextTLV(4, false, asn1ber.EncodeIntegerContent(10))...)

	// [5] initRequestDetail
	var detail5 []byte
	// [0] proposedVersionNumber: 1
	detail5 = append(detail5, asn1ber.EncodeContextTLV(0, false, asn1ber.EncodeIntegerContent(1))...)
	// [1] proposedParameterCBB: 16-bit BIT STRING - standard IEC 61850 value
	detail5 = append(detail5, asn1ber.EncodeContextTLV(1, false, []byte{0x03, 0xef, 0x18})...)
	// [2] servicesSupportedCalling: 11-byte BIT STRING
	servicesBits := []byte{
		0xee, 0x1c, 0x00, 0x00, 0x04, 0x08, 0x00, 0x00, 0x79, 0xef, 0x18,
	}
	detail5 = append(detail5, asn1ber.EncodeContextTLV(2, false, append([]byte{0x00}, servicesBits...))...)

	body = append(body, asn1ber.EncodeContextTLV(5, true, detail5)...)

	return append([]byte{tagInitiateRequestPDU}, append(asn1ber.EncodeLength(len(body)), body...)...)
}

// ParseInitiateResponse parses an MMS Initiate Response PDU and returns
// the negotiated max PDU size and outstanding calls.
func ParseInitiateResponse(buf []byte) (int32, int16, error) {
	if len(buf) < 2 {
		return 0, 0, fmt.Errorf("mms: initiate response too short")
	}
	if buf[0] != tagInitiateResponsePDU {
		return 0, 0, fmt.Errorf("mms: expected InitiateResponsePDU (0x%02X), got 0x%02X",
			tagInitiateResponsePDU, buf[0])
	}
	// Parse length and body
	length, offset, err := asn1ber.DecodeLength(buf, 1)
	if err != nil {
		return 0, 0, err
	}
	if offset+length > len(buf) {
		return 0, 0, fmt.Errorf("mms: initiate response truncated")
	}
	body := buf[offset : offset+length]

	var localDetail int32
	var maxOutstanding int16

	pos := 0
	for pos < len(body) {
		tlv, newPos, err := asn1ber.ParseTLV(body, pos)
		if err != nil {
			break
		}
		pos = newPos
		if tlv.Class == asn1ber.ClassContext {
			switch tlv.Tag {
			case 0: // localDetailCalled
				v, _ := asn1ber.DecodeInteger(tlv.Value)
				localDetail = int32(v)
			case 1: // negotiatedMaxServOutstandingCalling
				v, _ := asn1ber.DecodeInteger(tlv.Value)
				maxOutstanding = int16(v)
			}
		}
	}
	return localDetail, maxOutstanding, nil
}

// EncodeReadRequest builds the wire-format MMS Read Request PDU.
// invokeID is the sequence number for matching responses to requests.
func EncodeReadRequest(invokeID uint32, specs []VariableSpecification) []byte {
	var listBody []byte
	for _, spec := range specs {
		listBody = append(listBody, encodeVariableSpec(spec)...)
	}
	// [1] EXPLICIT variableAccessSpec { [0] listOfVariable { 0x30 items... } }
	listOfVar := asn1ber.EncodeContextTLV(0, true, listBody)
	varAccessSpec := asn1ber.EncodeContextTLV(1, true, listOfVar)
	svcTag := encodeContextImplicit(svcRead, true, varAccessSpec)
	return encodeConfirmedRequest(invokeID, svcTag)
}

// encodeVariableSpec encodes a VariableSpecification as a BER ListOfVariableSeq entry.
// Wire format: 0x30 { [0] EXPLICIT name { [1] domain-specific { 0x1a domainId 0x1a itemId } } }
func encodeVariableSpec(spec VariableSpecification) []byte {
	var domainBody []byte
	domainBody = append(domainBody, asn1ber.EncodeVisibleString(spec.DomainID)...)         // 0x1a
	domainBody = append(domainBody, asn1ber.EncodeVisibleString(spec.ItemID)...)           // 0x1a
	domainSpec := asn1ber.EncodeContextTLV(1, true, domainBody)                            // [1] domain-specific
	nameField := asn1ber.EncodeContextTLV(0, true, domainSpec)                             // [0] EXPLICIT name
	return asn1ber.EncodeTLV(asn1ber.ClassUniversal, true, asn1ber.TagSequence, nameField) // 0x30
}

// EncodeWriteRequest builds the wire-format MMS Write Request PDU.
func EncodeWriteRequest(invokeID uint32, specs []VariableSpecification, values []*Value) ([]byte, error) {
	var varsList []byte
	for _, spec := range specs {
		varsList = append(varsList, encodeVariableSpec(spec)...)
	}

	var valuesList []byte
	for _, v := range values {
		enc, err := EncodeValue(v)
		if err != nil {
			return nil, err
		}
		valuesList = append(valuesList, enc...) // no per-element wrapper
	}

	var svcBody []byte
	svcBody = append(svcBody, asn1ber.EncodeContextTLV(0, true, varsList)...)
	// listOfData uses [0] (same tag as listOfVariable; position disambiguates per ISO 9506)
	svcBody = append(svcBody, asn1ber.EncodeContextTLV(0, true, valuesList)...)

	svcTag := encodeContextImplicit(svcWrite, true, svcBody)
	return encodeConfirmedRequest(invokeID, svcTag), nil
}

// EncodeGetNameListRequest builds a GetNameList request PDU.
func EncodeGetNameListRequest(invokeID uint32, objectClass int, domainID string) []byte {
	var svcBody []byte

	// [0] objectClass: [0] basicObjectClass integer
	classContent := asn1ber.EncodeContextTLV(0, false, asn1ber.EncodeIntegerContent(int64(objectClass)))
	svcBody = append(svcBody, asn1ber.EncodeContextTLV(0, true, classContent)...)

	// [1] objectScope: [1] domainSpecific or [0] vmdSpecific
	if domainID != "" {
		scopeContent := asn1ber.EncodeContextTLV(1, false, []byte(domainID))
		svcBody = append(svcBody, asn1ber.EncodeContextTLV(1, true, scopeContent)...)
	} else {
		// [0] vmdSpecific: NULL
		svcBody = append(svcBody, asn1ber.EncodeContextTLV(1, true,
			asn1ber.EncodeContextTLV(0, false, nil))...)
	}

	svcTag := encodeContextImplicit(svcGetNameList, true, svcBody)
	return encodeConfirmedRequest(invokeID, svcTag)
}

// ParseConfirmedResponse parses the invokeID and service tag from a confirmed response PDU.
// Returns invokeID, service number, and the service-specific content bytes.
func ParseConfirmedResponse(buf []byte) (uint32, int, []byte, error) {
	if len(buf) < 2 || buf[0] != tagConfirmedResponse {
		return 0, 0, nil, fmt.Errorf("mms: expected ConfirmedResponsePDU, got 0x%02X", buf[0])
	}
	length, offset, err := asn1ber.DecodeLength(buf, 1)
	if err != nil {
		return 0, 0, nil, err
	}
	body := buf[offset : offset+length]

	// [0] invokeId
	tlv, pos, err := asn1ber.ParseTLV(body, 0)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("mms: parse invokeId: %w", err)
	}
	invokeID, err := asn1ber.DecodeUint32(tlv.Value)
	if err != nil {
		return 0, 0, nil, err
	}

	// [x] service response
	if pos >= len(body) {
		return invokeID, 0, nil, nil
	}
	svcTLV, _, err := asn1ber.ParseTLV(body, pos)
	if err != nil {
		return invokeID, 0, nil, fmt.Errorf("mms: parse service response: %w", err)
	}
	return invokeID, svcTLV.Tag, svcTLV.Value, nil
}

// ParseReadResponse parses the service content of a Read response.
// Wire: [1] IMPLICIT listOfAccessResults { AccessResult... }
// AccessResult = CHOICE { failure [1] IMPLICIT DataAccessError (0x81), success Data (direct tag) }
func ParseReadResponse(content []byte) ([]*ReadResult, error) {
	// Strip [1] listOfAccessResults wrapper
	listTLV, _, err := asn1ber.ParseTLV(content, 0)
	if err != nil {
		return nil, fmt.Errorf("mms: parse read response: %w", err)
	}
	if listTLV.Class != asn1ber.ClassContext || listTLV.Tag != 1 {
		return nil, fmt.Errorf("mms: read response: expected [1] listOfAccessResults, got 0x%02X", content[0])
	}

	var results []*ReadResult
	offset := 0
	for offset < len(listTLV.Value) {
		result := &ReadResult{}
		// Failure: [1] IMPLICIT INTEGER = primitive 0x81
		if listTLV.Value[offset] == 0x81 {
			tlv, newOff, err := asn1ber.ParseTLV(listTLV.Value, offset)
			if err != nil {
				return nil, fmt.Errorf("mms: parse access result failure: %w", err)
			}
			offset = newOff
			v, _ := asn1ber.DecodeInteger(tlv.Value)
			result.Error = DataAccessError(v)
			result.IsError = true
		} else {
			// Success: Data value encoded directly with its own tag
			v, newOff, err := DecodeValue(listTLV.Value, offset)
			if err != nil {
				return nil, fmt.Errorf("mms: parse access result value: %w", err)
			}
			offset = newOff
			result.Value = v
		}
		results = append(results, result)
	}
	return results, nil
}

// ParseGetNameListResponse parses a GetNameList response.
func ParseGetNameListResponse(content []byte) (*GetNameListResponse, error) {
	resp := &GetNameListResponse{}
	offset := 0
	for offset < len(content) {
		tlv, newOff, err := asn1ber.ParseTLV(content, offset)
		if err != nil {
			return nil, err
		}
		offset = newOff
		if tlv.Class == asn1ber.ClassContext {
			switch tlv.Tag {
			case 0: // listOfIdentifier
				innerOffset := 0
				for innerOffset < len(tlv.Value) {
					inner, newInner, err := asn1ber.ParseTLV(tlv.Value, innerOffset)
					if err != nil {
						break
					}
					innerOffset = newInner
					resp.Names = append(resp.Names, string(inner.Value))
				}
			case 1: // moreFollows: BOOLEAN
				if len(tlv.Value) > 0 {
					resp.MoreFollows = tlv.Value[0] != 0
				}
			}
		}
	}
	return resp, nil
}

// ParseConfirmedError parses a ConfirmedErrorPDU and returns the error.
func ParseConfirmedError(buf []byte) (uint32, Error, error) {
	if len(buf) < 2 || buf[0] != tagConfirmedError {
		return 0, 0, fmt.Errorf("mms: expected ConfirmedErrorPDU, got 0x%02X", buf[0])
	}
	length, offset, err := asn1ber.DecodeLength(buf, 1)
	if err != nil {
		return 0, 0, err
	}
	body := buf[offset : offset+length]

	var invokeID uint32
	var mmsErr Error

	pos := 0
	for pos < len(body) {
		tlv, newPos, err := asn1ber.ParseTLV(body, pos)
		if err != nil {
			break
		}
		pos = newPos
		if tlv.Class == asn1ber.ClassContext {
			switch tlv.Tag {
			case 0:
				v, _ := asn1ber.DecodeUint32(tlv.Value)
				invokeID = v
			case 1:
				// serviceError: contains errorClass and errorCode
				innerPos := 0
				for innerPos < len(tlv.Value) {
					inner, newInner, err := asn1ber.ParseTLV(tlv.Value, innerPos)
					if err != nil {
						break
					}
					innerPos = newInner
					if inner.Class == asn1ber.ClassContext {
						v, _ := asn1ber.DecodeInteger(inner.Value)
						mmsErr = Error(inner.Tag*10 + int(v))
					}
				}
			}
		}
	}
	return invokeID, mmsErr, nil
}

// ---- helper encoders ----

// encodeConfirmedRequest wraps a service PDU in a ConfirmedRequestPDU envelope.
func encodeConfirmedRequest(invokeID uint32, svcPDU []byte) []byte {
	var body []byte
	// invokeId: UNIVERSAL INTEGER (0x02), not context[0] — the C asn1c parser checks for 0x02
	body = append(body, asn1ber.EncodeUnsigned(invokeID)...)
	body = append(body, svcPDU...)

	length := asn1ber.EncodeLength(len(body))
	out := make([]byte, 1+len(length)+len(body))
	out[0] = tagConfirmedRequest
	copy(out[1:], length)
	copy(out[1+len(length):], body)
	return out
}

// encodeContextImplicit encodes a context-specific implicit tag.
func encodeContextImplicit(tag int, constructed bool, content []byte) []byte {
	b := byte(asn1ber.ClassContext | tag)
	if constructed {
		b |= asn1ber.Constructed
	}
	length := asn1ber.EncodeLength(len(content))
	out := make([]byte, 1+len(length)+len(content))
	out[0] = b
	copy(out[1:], length)
	copy(out[1+len(length):], content)
	return out
}

// ParsePDUType reads only the outer tag byte of an MMS PDU to determine its type.
func ParsePDUType(buf []byte) (byte, error) {
	if len(buf) == 0 {
		return 0, fmt.Errorf("mms: empty buffer")
	}
	return buf[0], nil
}

// ParseUnconfirmedPDU parses an Unconfirmed PDU (used for information reports).
func ParseUnconfirmedPDU(buf []byte) (int, []byte, error) {
	if len(buf) < 2 || buf[0] != tagUnconfirmedPDU {
		return 0, nil, fmt.Errorf("mms: expected UnconfirmedPDU, got 0x%02X", buf[0])
	}
	length, offset, err := asn1ber.DecodeLength(buf, 1)
	if err != nil {
		return 0, nil, err
	}
	body := buf[offset : offset+length]

	tlv, _, err := asn1ber.ParseTLV(body, 0)
	if err != nil {
		return 0, nil, err
	}
	return tlv.Tag, tlv.Value, nil
}

// EncodeConcludeRequest builds a ConcludeRequest PDU for orderly connection teardown.
func EncodeConcludeRequest() []byte {
	return []byte{tagConcludeRequestPDU, 0x00}
}
