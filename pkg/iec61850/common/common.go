/*
 *  common.go
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

// Package common defines IEC 61850 standard types shared by both client
// and server sides of the communication stack.
//
// Key concepts:
//   - Functional Constraint (FC): categorizes data attributes (e.g., ST=status, MX=measurands)
//   - Quality: validity and source information for measured values
//   - Timestamp: IEC 61850 UTC timestamp
//   - Control models: direct/SBO with normal/enhanced security
package common

import (
	"fmt"
	"strings"
)

// Edition constants for IEC 61850 standard editions.
const (
	Edition1   = 0
	Edition2   = 1
	Edition2_1 = 2
)

// FunctionalConstraint represents an IEC 61850 Functional Constraint (FC)
// as defined in IEC 61850-7-2.
type FunctionalConstraint int

const (
	// FC_ST Status information
	FC_ST FunctionalConstraint = 0
	// FC_MX Measurands - analog values
	FC_MX FunctionalConstraint = 1
	// FC_SP Setpoint
	FC_SP FunctionalConstraint = 2
	// FC_SV Substitution
	FC_SV FunctionalConstraint = 3
	// FC_CF Configuration
	FC_CF FunctionalConstraint = 4
	// FC_DC Description
	FC_DC FunctionalConstraint = 5
	// FC_SG Setting group
	FC_SG FunctionalConstraint = 6
	// FC_SE Setting group editable
	FC_SE FunctionalConstraint = 7
	// FC_SR Service response / Service tracking
	FC_SR FunctionalConstraint = 8
	// FC_OR Operate received
	FC_OR FunctionalConstraint = 9
	// FC_BL Blocking
	FC_BL FunctionalConstraint = 10
	// FC_EX Extended definition
	FC_EX FunctionalConstraint = 11
	// FC_CO Control
	FC_CO FunctionalConstraint = 12
	// FC_US Unicast SV
	FC_US FunctionalConstraint = 13
	// FC_MS Multicast SV
	FC_MS FunctionalConstraint = 14
	// FC_RP Unbuffered report
	FC_RP FunctionalConstraint = 15
	// FC_BR Buffered report
	FC_BR FunctionalConstraint = 16
	// FC_LG Log control blocks
	FC_LG FunctionalConstraint = 17
	// FC_GO Goose control blocks
	FC_GO FunctionalConstraint = 18
	// FC_ALL All FCs (wildcard)
	FC_ALL FunctionalConstraint = 99
	// FC_NONE No FC (invalid)
	FC_NONE FunctionalConstraint = -1
)

func (fc FunctionalConstraint) String() string {
	switch fc {
	case FC_ST:
		return "ST"
	case FC_MX:
		return "MX"
	case FC_SP:
		return "SP"
	case FC_SV:
		return "SV"
	case FC_CF:
		return "CF"
	case FC_DC:
		return "DC"
	case FC_SG:
		return "SG"
	case FC_SE:
		return "SE"
	case FC_SR:
		return "SR"
	case FC_OR:
		return "OR"
	case FC_BL:
		return "BL"
	case FC_EX:
		return "EX"
	case FC_CO:
		return "CO"
	case FC_US:
		return "US"
	case FC_MS:
		return "MS"
	case FC_RP:
		return "RP"
	case FC_BR:
		return "BR"
	case FC_LG:
		return "LG"
	case FC_GO:
		return "GO"
	case FC_ALL:
		return "ALL"
	default:
		return fmt.Sprintf("FC(%d)", int(fc))
	}
}

// ParseFC parses a functional constraint string (e.g., "ST", "MX") into a FunctionalConstraint.
func ParseFC(s string) FunctionalConstraint {
	switch strings.ToUpper(s) {
	case "ST":
		return FC_ST
	case "MX":
		return FC_MX
	case "SP":
		return FC_SP
	case "SV":
		return FC_SV
	case "CF":
		return FC_CF
	case "DC":
		return FC_DC
	case "SG":
		return FC_SG
	case "SE":
		return FC_SE
	case "SR":
		return FC_SR
	case "OR":
		return FC_OR
	case "BL":
		return FC_BL
	case "EX":
		return FC_EX
	case "CO":
		return FC_CO
	case "US":
		return FC_US
	case "MS":
		return FC_MS
	case "RP":
		return FC_RP
	case "BR":
		return FC_BR
	case "LG":
		return FC_LG
	case "GO":
		return FC_GO
	case "ALL", "*":
		return FC_ALL
	default:
		return FC_NONE
	}
}

// Quality represents the IEC 61850 quality bitstring (q) attribute.
// It encodes validity, source, and supplemental information.
type Quality uint16

const (
	// Quality validity bits (bits 0-1)
	QualityGood         Quality = 0x0000
	QualityInvalid      Quality = 0x0001
	QualityReserved     Quality = 0x0002
	QualityQuestionable Quality = 0x0003
	QualityValidityMask Quality = 0x0003

	// Quality source bits
	QualityOverflow        Quality = 0x0004
	QualityOutOfRange      Quality = 0x0008
	QualityBadReference    Quality = 0x0010
	QualityOscillatory     Quality = 0x0020
	QualityFailure         Quality = 0x0040
	QualityOldData         Quality = 0x0080
	QualityInconsistent    Quality = 0x0100
	QualityInaccurate      Quality = 0x0200
	QualitySource          Quality = 0x0400 // 0=process, 1=substituted
	QualityTest            Quality = 0x0800
	QualityOperatorBlocked Quality = 0x1000
)

// Validity returns the validity portion of the quality.
func (q Quality) Validity() Quality {
	return q & QualityValidityMask
}

// IsGood returns true if the quality is GOOD (valid, no issues).
func (q Quality) IsGood() bool {
	return q == QualityGood
}

// IsInvalid returns true if the validity bits indicate INVALID.
func (q Quality) IsInvalid() bool {
	return q.Validity() == QualityInvalid
}

// IsQuestionable returns true if the validity bits indicate QUESTIONABLE.
func (q Quality) IsQuestionable() bool {
	return q.Validity() == QualityQuestionable
}

// IsSubstituted returns true if the value is substituted (not from process).
func (q Quality) IsSubstituted() bool {
	return q&QualitySource != 0
}

// IsTest returns true if the test flag is set.
func (q Quality) IsTest() bool {
	return q&QualityTest != 0
}

// IsOperatorBlocked returns true if the operator-blocked flag is set.
func (q Quality) IsOperatorBlocked() bool {
	return q&QualityOperatorBlocked != 0
}

// ControlModel defines how a controllable data object is operated.
type ControlModel int

const (
	// ControlModelStatusOnly no control, status information only
	ControlModelStatusOnly ControlModel = 0
	// ControlModelDirectNormal direct control with normal security
	ControlModelDirectNormal ControlModel = 1
	// ControlModelSBONormal select-before-operate with normal security
	ControlModelSBONormal ControlModel = 2
	// ControlModelDirectEnhanced direct control with enhanced security
	ControlModelDirectEnhanced ControlModel = 3
	// ControlModelSBOEnhanced select-before-operate with enhanced security
	ControlModelSBOEnhanced ControlModel = 4
)

// TriggerOption defines when a report is triggered. Multiple options can be OR'd.
type TriggerOption uint8

const (
	TriggerDataChanged    TriggerOption = 0x01
	TriggerQualityChanged TriggerOption = 0x02
	TriggerDataUpdate     TriggerOption = 0x04
	TriggerIntegrity      TriggerOption = 0x08
	TriggerGI             TriggerOption = 0x10
	TriggerTransient      TriggerOption = 0x80
)

// ReportOption defines what is included in a report. Multiple options can be OR'd.
type ReportOption uint8

const (
	ReportOptSeqNum             ReportOption = 0x01
	ReportOptTimeStamp          ReportOption = 0x02
	ReportOptReasonForInclusion ReportOption = 0x04
	ReportOptDataSet            ReportOption = 0x08
	ReportOptDataReference      ReportOption = 0x10
	ReportOptBufferOverflow     ReportOption = 0x20
	ReportOptEntryID            ReportOption = 0x40
	ReportOptConfRev            ReportOption = 0x80
)

// ReasonForInclusion indicates why a data set member was included in a report.
type ReasonForInclusion int

const (
	ReasonNotIncluded        ReasonForInclusion = 0
	ReasonDataChange         ReasonForInclusion = 1
	ReasonQualityChange      ReasonForInclusion = 2
	ReasonDataUpdate         ReasonForInclusion = 3
	ReasonIntegrity          ReasonForInclusion = 4
	ReasonGI                 ReasonForInclusion = 5
	ReasonApplicationTrigger ReasonForInclusion = 6
)

// OriginatorCategory defines who initiated a control operation.
type OriginatorCategory int

const (
	OrCatNotSupported     OriginatorCategory = 0
	OrCatBayControl       OriginatorCategory = 1
	OrCatStationControl   OriginatorCategory = 2
	OrCatRemoteControl    OriginatorCategory = 3
	OrCatAutomaticBay     OriginatorCategory = 4
	OrCatAutomaticStation OriginatorCategory = 5
	OrCatAutomaticRemote  OriginatorCategory = 6
	OrCatMaintenance      OriginatorCategory = 7
	OrCatProcess          OriginatorCategory = 8
)

// ControlAddCause holds the additional cause for control model errors.
type ControlAddCause int

const (
	AddCauseUnknown                     ControlAddCause = 0
	AddCauseNotSupported                ControlAddCause = 1
	AddCauseBlockedBySwitchingHierarchy ControlAddCause = 2
	AddCauseSelectFailed                ControlAddCause = 3
	AddCauseInvalidPosition             ControlAddCause = 4
	AddCausePositionReached             ControlAddCause = 5
	AddCauseParameterChangeInExecution  ControlAddCause = 6
	AddCauseStepLimit                   ControlAddCause = 7
	AddCauseBlockedByMode               ControlAddCause = 8
	AddCauseBlockedByProcess            ControlAddCause = 9
	AddCauseBlockedByInterlocking       ControlAddCause = 10
	AddCauseBlockedBySynchrocheck       ControlAddCause = 11
	AddCauseCommandAlreadyInExecution   ControlAddCause = 12
	AddCauseBlockedByHealth             ControlAddCause = 13
	AddCause1OfNControl                 ControlAddCause = 14
	AddCauseAbortionByCancel            ControlAddCause = 15
	AddCauseTimeLimitOver               ControlAddCause = 16
	AddCauseAbortionByTrip              ControlAddCause = 17
	AddCauseObjectNotSelected           ControlAddCause = 18
	AddCauseObjectAlreadySelected       ControlAddCause = 19
	AddCauseNoAccessAuthority           ControlAddCause = 20
	AddCauseEndedWithOvershoot          ControlAddCause = 21
	AddCauseAbortionDueToDeviation      ControlAddCause = 22
	AddCauseAbortionByCommunicationLoss ControlAddCause = 23
	AddCauseAbortionByCommand           ControlAddCause = 24
	AddCauseNone                        ControlAddCause = 25
	AddCauseInconsistentParameters      ControlAddCause = 26
	AddCauseLockedByOtherClient         ControlAddCause = 27
)

// ACSIClass represents an IEC 61850 ACSI service class.
type ACSIClass int

const (
	ACSIClassDataObject ACSIClass = iota
	ACSIClassDataSet
	ACSIClassBRCB // Buffered Report Control Block
	ACSIClassURCB // Unbuffered Report Control Block
	ACSIClassLCB  // Log Control Block
	ACSIClassLog
	ACSIClassSGCB  // Setting Group Control Block
	ACSIClassGoCB  // GOOSE Control Block
	ACSIClassGsCB  // GSSE Control Block
	ACSIClassMSVCB // Multicast Sampled Value CB
	ACSIClassUSVCB // Unicast Sampled Value CB
)

// PhyComAddress holds Ethernet/VLAN communication parameters for GOOSE/SV.
type PhyComAddress struct {
	VLANPriority uint8
	VLANID       uint16
	AppID        uint16
	DstAddress   [6]byte
}

// DefaultGooseMulticastAddress returns the default GOOSE multicast MAC address.
func DefaultGooseMulticastAddress() [6]byte {
	return [6]byte{0x01, 0x0C, 0xCD, 0x01, 0x00, 0x00}
}

// DefaultSVMulticastAddress returns the default Sampled Values multicast MAC address.
func DefaultSVMulticastAddress() [6]byte {
	return [6]byte{0x01, 0x0C, 0xCD, 0x04, 0x00, 0x00}
}

// ObjectReference represents a full IEC 61850 object reference.
// Format: LDName/LNName.DOName[.DAName][.SubDA...][$FC]
type ObjectReference struct {
	LogicalDevice string
	LogicalNode   string
	DataObject    string
	DataAttribute string
	FC            FunctionalConstraint
}

// ParseObjectReference parses an IEC 61850 object reference string.
// Accepted format: "LDName/LNClass.DOName[.DAName][$FC]"
func ParseObjectReference(ref string) (ObjectReference, error) {
	or := ObjectReference{FC: FC_NONE}

	// Split on '$' to extract FC
	parts := strings.SplitN(ref, "$", 2)
	if len(parts) == 2 {
		or.FC = ParseFC(parts[1])
		ref = parts[0]
	}

	// Split on '/' to separate LD from LN.DO.DA
	slashIdx := strings.IndexByte(ref, '/')
	if slashIdx < 0 {
		return or, fmt.Errorf("iec61850: missing '/' in object reference %q", ref)
	}
	or.LogicalDevice = ref[:slashIdx]
	rest := ref[slashIdx+1:]

	// Split on '.' to get LN, DO, DA
	dotParts := strings.SplitN(rest, ".", 3)
	if len(dotParts) < 2 {
		return or, fmt.Errorf("iec61850: expected LN.DO in %q", rest)
	}
	or.LogicalNode = dotParts[0]
	or.DataObject = dotParts[1]
	if len(dotParts) == 3 {
		or.DataAttribute = dotParts[2]
	}
	return or, nil
}

// String returns the canonical IEC 61850 object reference string.
func (or ObjectReference) String() string {
	s := or.LogicalDevice + "/" + or.LogicalNode + "." + or.DataObject
	if or.DataAttribute != "" {
		s += "." + or.DataAttribute
	}
	if or.FC != FC_NONE && or.FC != FC_ALL {
		s += "$" + or.FC.String()
	}
	return s
}

// MMSObjectReference converts an IEC 61850 object reference to the MMS
// domain-specific variable name used on the wire.
// Format: domainId = LDName, itemId = LNName$FC$DOName[$DAName]
func (or ObjectReference) MMSObjectReference() (domainID, itemID string) {
	domainID = or.LogicalDevice
	if or.FC == FC_NONE || or.FC == FC_ALL {
		itemID = or.LogicalNode + "." + or.DataObject
		if or.DataAttribute != "" {
			itemID += "." + or.DataAttribute
		}
	} else {
		itemID = or.LogicalNode + "$" + or.FC.String() + "$" + or.DataObject
		if or.DataAttribute != "" {
			itemID += "$" + or.DataAttribute
		}
	}
	return
}

// DataAttributeType defines the primitive data type of an IEC 61850 data attribute.
type DataAttributeType int

const (
	TypeUnknown       DataAttributeType = -1
	TypeBoolean       DataAttributeType = 0
	TypeINT8          DataAttributeType = 1
	TypeINT16         DataAttributeType = 2
	TypeINT32         DataAttributeType = 3
	TypeINT64         DataAttributeType = 4
	TypeINT128        DataAttributeType = 5
	TypeINT8U         DataAttributeType = 6
	TypeINT16U        DataAttributeType = 7
	TypeINT24U        DataAttributeType = 8
	TypeINT32U        DataAttributeType = 9
	TypeFLOAT32       DataAttributeType = 10
	TypeFLOAT64       DataAttributeType = 11
	TypeEnumerated    DataAttributeType = 12
	TypeOctetString64 DataAttributeType = 13
	TypeOctetString6  DataAttributeType = 14
	TypeOctetString8  DataAttributeType = 15
	TypeVisibleStr32  DataAttributeType = 16
	TypeVisibleStr64  DataAttributeType = 17
	TypeVisibleStr65  DataAttributeType = 18
	TypeVisibleStr129 DataAttributeType = 19
	TypeVisibleStr255 DataAttributeType = 20
	TypeUnicodeStr255 DataAttributeType = 21
	TypeTimestamp     DataAttributeType = 22
	TypeQuality       DataAttributeType = 23
	TypeCheck         DataAttributeType = 24
	TypeCodedEnum     DataAttributeType = 25
	TypeGenericBitStr DataAttributeType = 26
	TypeConstructed   DataAttributeType = 27
	TypeEntryTime     DataAttributeType = 28
	TypePhyComAddr    DataAttributeType = 29
	TypeCurrency      DataAttributeType = 30
	TypeOptFlds       DataAttributeType = 31
	TypeTrgOps        DataAttributeType = 32
)

// IedClientError mirrors the IED_ERROR_* codes from the C library.
type IedClientError int

const (
	ErrorOK                                IedClientError = 0
	ErrorNotConnected                      IedClientError = 1
	ErrorAlreadyConnected                  IedClientError = 2
	ErrorConnectionLost                    IedClientError = 3
	ErrorServiceNotSupported               IedClientError = 4
	ErrorConnectionRejected                IedClientError = 5
	ErrorOutstandingCallLimitReached       IedClientError = 6
	ErrorUserProvidedInvalidArgument       IedClientError = 10
	ErrorEnableReportFailedDatasetMismatch IedClientError = 11
	ErrorObjectReferenceInvalid            IedClientError = 12
	ErrorUnexpectedValueReceived           IedClientError = 13
	ErrorTimeout                           IedClientError = 20
	ErrorAccessDenied                      IedClientError = 21
	ErrorObjectDoesNotExist                IedClientError = 22
	ErrorObjectExists                      IedClientError = 23
	ErrorObjectAccessUnsupported           IedClientError = 24
	ErrorTypeInconsistent                  IedClientError = 25
	ErrorTemporarilyUnavailable            IedClientError = 26
	ErrorObjectUndefined                   IedClientError = 27
	ErrorInvalidAddress                    IedClientError = 28
	ErrorHardwareFault                     IedClientError = 29
	ErrorTypeUnsupported                   IedClientError = 30
	ErrorObjectAttributeInconsistent       IedClientError = 31
	ErrorObjectValueInvalid                IedClientError = 32
	ErrorObjectInvalidated                 IedClientError = 33
	ErrorMalformedMessage                  IedClientError = 34
	ErrorObjectConstraintConflict          IedClientError = 35
	ErrorServiceNotImplemented             IedClientError = 98
	ErrorUnknown                           IedClientError = 99
)

func (e IedClientError) Error() string {
	switch e {
	case ErrorOK:
		return "ok"
	case ErrorNotConnected:
		return "not connected"
	case ErrorConnectionLost:
		return "connection lost"
	case ErrorTimeout:
		return "timeout"
	case ErrorAccessDenied:
		return "access denied"
	case ErrorObjectDoesNotExist:
		return "object does not exist"
	case ErrorObjectExists:
		return "object already exists"
	case ErrorConnectionRejected:
		return "connection rejected"
	default:
		return fmt.Sprintf("IedClientError(%d)", int(e))
	}
}
