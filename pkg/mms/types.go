/*
 *  types.go
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

// Package mms implements the Manufacturing Message Specification (MMS) protocol
// as defined in ISO 9506, used as the communication protocol for IEC 61850.
//
// MMS defines a set of services for reading/writing variables, managing named
// variable lists (data sets), and other industrial automation tasks.
// Data is encoded using ASN.1 BER.
package mms

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// Type represents the MMS data type of a Value.
type Type int

const (
	TypeArray           Type = 0
	TypeStructure       Type = 1
	TypeBoolean         Type = 2
	TypeBitString       Type = 3
	TypeInteger         Type = 4
	TypeUnsigned        Type = 5
	TypeFloat           Type = 6
	TypeOctetString     Type = 7
	TypeVisibleString   Type = 8
	TypeGeneralizedTime Type = 9
	TypeBinaryTime      Type = 10
	TypeBCD             Type = 11
	TypeObjID           Type = 12
	TypeString          Type = 13 // MMS unicode string
	TypeUTCTime         Type = 14
	TypeDataAccessError Type = 15
)

func (t Type) String() string {
	names := [...]string{
		"ARRAY", "STRUCTURE", "BOOLEAN", "BIT-STRING", "INTEGER",
		"UNSIGNED", "FLOAT", "OCTET-STRING", "VISIBLE-STRING",
		"GENERALIZED-TIME", "BINARY-TIME", "BCD", "OBJ-ID",
		"MMS-STRING", "UTC-TIME", "DATA-ACCESS-ERROR",
	}
	if int(t) < len(names) {
		return names[t]
	}
	return fmt.Sprintf("Type(%d)", int(t))
}

// DataAccessError represents an MMS data access error code.
type DataAccessError int

const (
	DataAccessErrorObjectInvalidated           DataAccessError = 0
	DataAccessErrorHardwareFault               DataAccessError = 1
	DataAccessErrorTemporarilyUnavailable      DataAccessError = 2
	DataAccessErrorObjectAccessDenied          DataAccessError = 3
	DataAccessErrorObjectUndefined             DataAccessError = 4
	DataAccessErrorInvalidAddress              DataAccessError = 5
	DataAccessErrorTypeUnsupported             DataAccessError = 6
	DataAccessErrorTypeInconsistent            DataAccessError = 7
	DataAccessErrorObjectAttributeInconsistent DataAccessError = 8
	DataAccessErrorObjectAccessUnsupported     DataAccessError = 9
	DataAccessErrorObjectNonExistent           DataAccessError = 10
	DataAccessErrorObjectValueInvalid          DataAccessError = 11
	DataAccessErrorUnknown                     DataAccessError = 12
	DataAccessErrorSuccessNoUpdate             DataAccessError = -3
	DataAccessErrorNoResponse                  DataAccessError = -2
	DataAccessErrorSuccess                     DataAccessError = -1
)

// Error represents an MMS protocol-level error.
type Error int

const (
	ErrNone                          Error = 0
	ErrConnectionRejected            Error = 1
	ErrConnectionLost                Error = 2
	ErrServiceTimeout                Error = 3
	ErrParsingResponse               Error = 4
	ErrHardwareFault                 Error = 5
	ErrConcludeRejected              Error = 6
	ErrInvalidArguments              Error = 7
	ErrOutstandingCallLimit          Error = 8
	ErrOther                         Error = 9
	ErrDefinitionInvalidAddress      Error = 31
	ErrDefinitionTypeUnsupported     Error = 32
	ErrDefinitionTypeInconsistent    Error = 33
	ErrDefinitionObjectUndefined     Error = 34
	ErrDefinitionObjectExists        Error = 35
	ErrAccessObjectNonExistent       Error = 81
	ErrAccessObjectAccessUnsupported Error = 82
	ErrAccessObjectAccessDenied      Error = 83
	ErrAccessObjectInvalidated       Error = 84
	ErrAccessObjectValueInvalid      Error = 85
	ErrAccessTemporarilyUnavailable  Error = 86
)

func (e Error) Error() string {
	switch e {
	case ErrNone:
		return "no error"
	case ErrConnectionRejected:
		return "connection rejected"
	case ErrConnectionLost:
		return "connection lost"
	case ErrServiceTimeout:
		return "service timeout"
	case ErrAccessObjectNonExistent:
		return "object does not exist"
	case ErrAccessObjectAccessDenied:
		return "access denied"
	default:
		return fmt.Sprintf("MMS error %d", int(e))
	}
}

// UTCTime represents an IEC 61850 timestamp with sub-second precision.
// The IEC 61850 timestamp is 8 bytes: 4 bytes unix seconds + 3 bytes fractions + 1 byte quality.
type UTCTime struct {
	Seconds              uint32
	Fractions            uint32 // 24-bit sub-second fractions (1/2^24 second resolution)
	AccuracyClass        byte
	ClockFailure         bool
	ClockNotSynchronized bool
	LeapSecondsKnown     bool
}

// ToTime converts a UTCTime to a time.Time (UTC).
func (u UTCTime) ToTime() time.Time {
	ns := int64(u.Fractions) * 1e9 / (1 << 24)
	return time.Unix(int64(u.Seconds), ns).UTC()
}

func (u UTCTime) QualityDecode() string {
	var flags []string

	if u.LeapSecondsKnown {
		flags = append(flags, "LeapSecondKnown")
	}
	if u.ClockFailure {
		flags = append(flags, "ClockFailure")
	}
	if u.ClockNotSynchronized {
		flags = append(flags, "ClockNotSychronized")
	}
	// (0x1F) - TimeAccuracy class

	if u.AccuracyClass > 0 {
		accuracy := getTimeAccuracy(u.AccuracyClass)
		flags = append(flags, fmt.Sprintf("T%d:%s", getPerformanceClass(u.AccuracyClass), accuracy))
	}

	if len(flags) == 0 {
		return "GOOD"
	}

	return strings.Join(flags, " ")
}

// UTCTimeFromTime creates a UTCTime from a time.Time.
func UTCTimeFromTime(t time.Time) UTCTime {
	t = t.UTC()
	fracs := uint32(float64(t.Nanosecond()) / 1e9 * (1 << 24))
	return UTCTime{
		Seconds:   uint32(t.Unix()),
		Fractions: fracs,
	}
}

// BinaryTime represents an MMS BinaryTime (either 4 or 6 bytes).
type BinaryTime struct {
	Milliseconds  uint32 // milliseconds since midnight
	DaysSince1984 uint16 // days since 1 Jan 1984 (only in 6-byte form)
	Size          int    // 4 or 6
}

// Value is the fundamental MMS data type that can hold any MMS value.
// It mirrors the sMmsValue structure in the C library.
type Value struct {
	typ Type

	// scalar values
	boolVal    bool
	intVal     int64
	floatVal   float64
	bitStr     []byte
	bitStrSize int // number of valid bits
	octetStr   []byte
	strVal     string
	utcTime    UTCTime
	binTime    BinaryTime
	dataErr    DataAccessError
	objID      []int // OID arcs

	// composite values
	elements []*Value
}

// NewBoolean creates a boolean Value.
func NewBoolean(v bool) *Value {
	return &Value{typ: TypeBoolean, boolVal: v}
}

// NewInt32 creates a signed 32-bit integer Value.
func NewInt32(v int32) *Value {
	return &Value{typ: TypeInteger, intVal: int64(v)}
}

// NewInt64 creates a signed 64-bit integer Value.
func NewInt64(v int64) *Value {
	return &Value{typ: TypeInteger, intVal: v}
}

// NewUint32 creates an unsigned 32-bit integer Value.
func NewUint32(v uint32) *Value {
	return &Value{typ: TypeUnsigned, intVal: int64(v)}
}

// NewFloat32 creates a 32-bit float Value.
func NewFloat32(v float32) *Value {
	return &Value{typ: TypeFloat, floatVal: float64(v)}
}

// NewFloat64 creates a 64-bit float Value.
func NewFloat64(v float64) *Value {
	return &Value{typ: TypeFloat, floatVal: v}
}

// NewVisibleString creates a visible string Value.
func NewVisibleString(s string) *Value {
	return &Value{typ: TypeVisibleString, strVal: s}
}

// NewOctetString creates an octet string Value from a byte slice.
func NewOctetString(data []byte) *Value {
	cp := make([]byte, len(data))
	copy(cp, data)
	return &Value{typ: TypeOctetString, octetStr: cp}
}

// NewBitString creates a bit string Value.
// bits is the byte slice holding the bits, numBits is the count of valid bits.
func NewBitString(bits []byte, numBits int) *Value {
	cp := make([]byte, len(bits))
	copy(cp, bits)
	return &Value{typ: TypeBitString, bitStr: cp, bitStrSize: numBits}
}

// NewUTCTime creates a UTC time Value.
func NewUTCTime(t UTCTime) *Value {
	return &Value{typ: TypeUTCTime, utcTime: t}
}

// NewBinaryTime creates a binary time Value.
func NewBinaryTime(sixByte bool) *Value {
	size := 4
	if sixByte {
		size = 6
	}
	return &Value{typ: TypeBinaryTime, binTime: BinaryTime{Size: size}}
}

// NewArray creates an array Value with the given elements.
func NewArray(elements []*Value) *Value {
	return &Value{typ: TypeArray, elements: elements}
}

// NewStructure creates a structure Value with the given members.
func NewStructure(members []*Value) *Value {
	return &Value{typ: TypeStructure, elements: members}
}

// NewDataAccessError creates a data access error Value.
func NewDataAccessError(code DataAccessError) *Value {
	return &Value{typ: TypeDataAccessError, dataErr: code}
}

// Type returns the MMS type of the value.
func (v *Value) Type() Type {
	return v.typ
}

// GetBoolean returns the boolean value. Panics if type is not TypeBoolean.
func (v *Value) GetBoolean() bool {
	if v.typ != TypeBoolean {
		panic(fmt.Sprintf("mms: GetBoolean called on %s value", v.typ))
	}
	return v.boolVal
}

// GetInt32 returns the integer as int32.
func (v *Value) GetInt32() int32 {
	if v.typ != TypeInteger && v.typ != TypeUnsigned {
		panic(fmt.Sprintf("mms: GetInt32 called on %s value", v.typ))
	}
	return int32(v.intVal)
}

// GetInt64 returns the integer as int64.
func (v *Value) GetInt64() int64 {
	if v.typ != TypeInteger && v.typ != TypeUnsigned {
		panic(fmt.Sprintf("mms: GetInt64 called on %s value", v.typ))
	}
	return v.intVal
}

// GetUint32 returns the value as uint32.
func (v *Value) GetUint32() uint32 {
	if v.typ != TypeInteger && v.typ != TypeUnsigned {
		panic(fmt.Sprintf("mms: GetUint32 called on %s value", v.typ))
	}
	return uint32(v.intVal)
}

// GetFloat32 returns the value as float32.
func (v *Value) GetFloat32() float32 {
	if v.typ != TypeFloat {
		panic(fmt.Sprintf("mms: GetFloat32 called on %s value", v.typ))
	}
	return float32(v.floatVal)
}

// GetFloat64 returns the value as float64.
func (v *Value) GetFloat64() float64 {
	if v.typ != TypeFloat {
		panic(fmt.Sprintf("mms: GetFloat64 called on %s value", v.typ))
	}
	return v.floatVal
}

// GetVisibleString returns the string value.
func (v *Value) GetVisibleString() string {
	if v.typ != TypeVisibleString && v.typ != TypeString {
		panic(fmt.Sprintf("mms: GetVisibleString called on %s value", v.typ))
	}
	return v.strVal
}

// GetOctetString returns the octet string as a byte slice.
func (v *Value) GetOctetString() []byte {
	if v.typ != TypeOctetString {
		panic(fmt.Sprintf("mms: GetOctetString called on %s value", v.typ))
	}
	cp := make([]byte, len(v.octetStr))
	copy(cp, v.octetStr)
	return cp
}

// GetUTCTime returns the UTC time value.
func (v *Value) GetUTCTime() UTCTime {
	if v.typ != TypeUTCTime {
		panic(fmt.Sprintf("mms: GetUTCTime called on %s value", v.typ))
	}
	return v.utcTime
}

// GetBitString returns the bit string bytes and bit count.
func (v *Value) GetBitString() ([]byte, int) {
	if v.typ != TypeBitString {
		panic(fmt.Sprintf("mms: GetBitString called on %s value", v.typ))
	}
	cp := make([]byte, len(v.bitStr))
	copy(cp, v.bitStr)
	return cp, v.bitStrSize
}

// GetBit returns the value of a single bit in a bit string.
func (v *Value) GetBit(index int) bool {
	if v.typ != TypeBitString {
		panic(fmt.Sprintf("mms: GetBit called on %s value", v.typ))
	}
	if index < 0 || index >= v.bitStrSize {
		return false
	}
	byteIdx := index / 8
	bitIdx := 7 - (index % 8)
	return (v.bitStr[byteIdx]>>uint(bitIdx))&1 == 1
}

// SetBit sets or clears a single bit in a bit string Value.
func (v *Value) SetBit(index int, set bool) {
	if v.typ != TypeBitString {
		panic(fmt.Sprintf("mms: SetBit called on %s value", v.typ))
	}
	if index < 0 || index >= v.bitStrSize {
		return
	}
	byteIdx := index / 8
	bitIdx := 7 - (index % 8)
	if set {
		v.bitStr[byteIdx] |= 1 << uint(bitIdx)
	} else {
		v.bitStr[byteIdx] &^= 1 << uint(bitIdx)
	}
}

// GetDataAccessError returns the error code for a TypeDataAccessError value.
func (v *Value) GetDataAccessError() DataAccessError {
	if v.typ != TypeDataAccessError {
		panic(fmt.Sprintf("mms: GetDataAccessError called on %s value", v.typ))
	}
	return v.dataErr
}

// GetElement returns element at index for TypeArray or TypeStructure.
func (v *Value) GetElement(index int) *Value {
	if v.typ != TypeArray && v.typ != TypeStructure {
		panic(fmt.Sprintf("mms: GetElement called on %s value", v.typ))
	}
	if index < 0 || index >= len(v.elements) {
		return nil
	}
	return v.elements[index]
}

// SetElement sets an element in an array or structure.
func (v *Value) SetElement(index int, elem *Value) {
	if v.typ != TypeArray && v.typ != TypeStructure {
		panic(fmt.Sprintf("mms: SetElement called on %s value", v.typ))
	}
	if index < 0 || index >= len(v.elements) {
		return
	}
	v.elements[index] = elem
}

// Size returns the number of elements in an array or structure.
func (v *Value) Size() int {
	if v.typ == TypeArray || v.typ == TypeStructure {
		return len(v.elements)
	}
	return 0
}

// Clone creates a deep copy of the Value.
func (v *Value) Clone() *Value {
	if v == nil {
		return nil
	}
	cp := &Value{
		typ:        v.typ,
		boolVal:    v.boolVal,
		intVal:     v.intVal,
		floatVal:   v.floatVal,
		bitStrSize: v.bitStrSize,
		strVal:     v.strVal,
		utcTime:    v.utcTime,
		binTime:    v.binTime,
		dataErr:    v.dataErr,
	}
	if v.bitStr != nil {
		cp.bitStr = make([]byte, len(v.bitStr))
		copy(cp.bitStr, v.bitStr)
	}
	if v.octetStr != nil {
		cp.octetStr = make([]byte, len(v.octetStr))
		copy(cp.octetStr, v.octetStr)
	}
	if v.objID != nil {
		cp.objID = make([]int, len(v.objID))
		copy(cp.objID, v.objID)
	}
	if v.elements != nil {
		cp.elements = make([]*Value, len(v.elements))
		for i, e := range v.elements {
			cp.elements[i] = e.Clone()
		}
	}
	return cp
}

// String returns a human-readable representation of the Value.
// Structures and arrays are expanded recursively.
func (v *Value) String() string {
	return v.stringIndent(0)
}

func (v *Value) stringIndent(depth int) string {
	switch v.typ {
	case TypeBoolean:
		return fmt.Sprintf("BOOLEAN(%v)", v.boolVal)
	case TypeInteger:
		return fmt.Sprintf("INTEGER(%d)", v.intVal)
	case TypeUnsigned:
		return fmt.Sprintf("UNSIGNED(%d)", uint64(v.intVal))
	case TypeFloat:
		if v.floatVal == float64(float32(v.floatVal)) {
			return fmt.Sprintf("FLOAT(%g)", float32(v.floatVal))
		}
		return fmt.Sprintf("FLOAT(%g)", v.floatVal)
	case TypeVisibleString:
		return fmt.Sprintf("VISIBLE-STRING(%q)", v.strVal)
	case TypeString:
		return fmt.Sprintf("MMS-STRING(%q)", v.strVal)
	case TypeOctetString:
		return fmt.Sprintf("OCTET-STRING(%X)", v.octetStr)
	case TypeBitString:
		return fmt.Sprintf("BIT-STRING(%d bits)", v.bitStrSize)
	case TypeUTCTime:
		return fmt.Sprintf("UTC-TIME(%s)", v.utcTime.ToTime().Format(time.RFC3339Nano))
	case TypeBinaryTime:
		return fmt.Sprintf("BINARY-TIME(%dms)", v.binTime.Milliseconds)
	case TypeDataAccessError:
		return fmt.Sprintf("DATA-ACCESS-ERROR(%d)", v.dataErr)
	case TypeArray:
		if len(v.elements) == 0 {
			return "ARRAY[]"
		}
		indent := indentStr(depth + 1)
		s := fmt.Sprintf("ARRAY[%d]{\n", len(v.elements))
		for i, e := range v.elements {
			s += fmt.Sprintf("%s[%d]: %s\n", indent, i, e.stringIndent(depth+1))
		}
		s += indentStr(depth) + "}"
		return s
	case TypeStructure:
		if len(v.elements) == 0 {
			return "STRUCTURE{}"
		}
		indent := indentStr(depth + 1)
		s := fmt.Sprintf("STRUCTURE{\n")
		for i, e := range v.elements {
			s += fmt.Sprintf("%s[%d]: %s\n", indent, i, e.stringIndent(depth+1))
		}
		s += indentStr(depth) + "}"
		return s
	default:
		return fmt.Sprintf("Value{type=%s}", v.typ)
	}
}

func indentStr(depth int) string {
	const tab = "  "
	s := ""
	for i := 0; i < depth; i++ {
		s += tab
	}
	return s
}

// IsNaN returns true if the float value is NaN.
func (v *Value) IsNaN() bool {
	return v.typ == TypeFloat && math.IsNaN(v.floatVal)
}
