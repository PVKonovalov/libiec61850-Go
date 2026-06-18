/*
 *  encode.go
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
	"encoding/binary"
	"fmt"
	"math"

	"github.com/PVKonovalov/libiec61850-Go/pkg/asn1ber"
)

// MMS ASN.1 implicit context tags for Data type (ISO 9506-2 A.2)
const (
	tagMMSBoolean       = 0x83 // [3] IMPLICIT BOOLEAN (primitive)
	tagMMSBitString     = 0x84 // [4] IMPLICIT BIT STRING
	tagMMSInteger       = 0x85 // [5] IMPLICIT INTEGER
	tagMMSUnsigned      = 0x86 // [6] IMPLICIT INTEGER (unsigned)
	tagMMSFloat         = 0x87 // [7] IMPLICIT OCTET STRING (float encoding)
	tagMMSOctetString   = 0x89 // [9] IMPLICIT OCTET STRING
	tagMMSVisibleString = 0x8A // [10] IMPLICIT VisibleString
	tagMMSGeneralTime   = 0x8B // [11]
	tagMMSBinaryTime    = 0x8C // [12]
	tagMMSBCD           = 0x8D // [13]
	tagMMSObjID         = 0x8E // [14]
	tagMMSString        = 0x90 // [16] IMPLICIT UTF8String
	tagMMSUTCTime       = 0x91 // [17] IMPLICIT OCTET STRING (8 bytes)

	// Constructed tags
	tagMMSArrayCons     = 0xA1 // [1] CONSTRUCTED = array
	tagMMSStructureCons = 0xA2 // [2] CONSTRUCTED = structure
)

// EncodeValue encodes an MMS Value into its BER wire representation.
// The encoding follows ISO 9506-2 section A.2 (Data).
func EncodeValue(v *Value) ([]byte, error) {
	if v == nil {
		return nil, fmt.Errorf("mms: cannot encode nil value")
	}
	switch v.typ {
	case TypeBoolean:
		b := byte(0x00)
		if v.boolVal {
			b = 0xFF
		}
		return []byte{tagMMSBoolean, 0x01, b}, nil

	case TypeInteger:
		content := asn1ber.EncodeIntegerContent(v.intVal)
		return encodePrimitive(tagMMSInteger, content), nil

	case TypeUnsigned:
		content := encodeUnsignedContent(uint64(v.intVal))
		return encodePrimitive(tagMMSUnsigned, content), nil

	case TypeFloat:
		// MMS float encoding: [exponent_width_byte][IEEE754 bytes...]
		// float32: 5 bytes total (exponent=8, 4 float bytes)
		// float64: 9 bytes total (exponent=11, 8 float bytes)
		f32 := float32(v.floatVal)
		if float64(f32) == v.floatVal {
			content := make([]byte, 5)
			content[0] = 8 // exponent bits for IEEE 754 single
			binary.BigEndian.PutUint32(content[1:], math.Float32bits(f32))
			return encodePrimitive(tagMMSFloat, content), nil
		}
		content := make([]byte, 9)
		content[0] = 11 // exponent bits for IEEE 754 double
		binary.BigEndian.PutUint64(content[1:], math.Float64bits(v.floatVal))
		return encodePrimitive(tagMMSFloat, content), nil

	case TypeBitString:
		numBytes := (v.bitStrSize + 7) / 8
		unused := 8*numBytes - v.bitStrSize
		content := make([]byte, 1+numBytes)
		content[0] = byte(unused)
		copy(content[1:], v.bitStr)
		return encodePrimitive(tagMMSBitString, content), nil

	case TypeOctetString:
		return encodePrimitive(tagMMSOctetString, v.octetStr), nil

	case TypeVisibleString:
		return encodePrimitive(tagMMSVisibleString, []byte(v.strVal)), nil

	case TypeString:
		return encodePrimitive(tagMMSString, []byte(v.strVal)), nil

	case TypeUTCTime:
		content := encodeUTCTime(v.utcTime)
		return encodePrimitive(tagMMSUTCTime, content), nil

	case TypeBinaryTime:
		content := encodeBinaryTime(v.binTime)
		return encodePrimitive(tagMMSBinaryTime, content), nil

	case TypeArray:
		body, err := encodeElements(v.elements)
		if err != nil {
			return nil, err
		}
		return encodeConstructed(tagMMSArrayCons, body), nil

	case TypeStructure:
		body, err := encodeElements(v.elements)
		if err != nil {
			return nil, err
		}
		return encodeConstructed(tagMMSStructureCons, body), nil

	case TypeDataAccessError:
		// AccessResult::failure [0] DataAccessError — tag 0x80 per ISO 9506-2
		return []byte{0x80, 0x01, byte(v.dataErr)}, nil

	default:
		return nil, fmt.Errorf("mms: unsupported type %s", v.typ)
	}
}

// DecodeValue decodes an MMS Value from BER bytes starting at offset.
// Returns the decoded Value and the new offset.
func DecodeValue(buf []byte, offset int) (*Value, int, error) {
	if offset >= len(buf) {
		return nil, offset, fmt.Errorf("mms: buffer empty at offset %d", offset)
	}
	tag := buf[offset]
	offset++

	length, offset, err := asn1ber.DecodeLength(buf, offset)
	if err != nil {
		return nil, offset, err
	}
	if offset+length > len(buf) {
		return nil, offset, fmt.Errorf("mms: value extends beyond buffer (need %d, have %d)", length, len(buf)-offset)
	}
	content := buf[offset : offset+length]
	offset += length

	switch tag {
	case tagMMSBoolean:
		if len(content) == 0 {
			return nil, offset, fmt.Errorf("mms: empty boolean")
		}
		return NewBoolean(content[0] != 0), offset, nil

	case tagMMSInteger:
		v, err := asn1ber.DecodeInteger(content)
		if err != nil {
			return nil, offset, err
		}
		return NewInt64(v), offset, nil

	case tagMMSUnsigned:
		var uv uint64
		for _, b := range content {
			uv = (uv << 8) | uint64(b)
		}
		return NewUint32(uint32(uv)), offset, nil

	case tagMMSFloat:
		// content[0] = exponent bit width; content[1:] = IEEE754 bytes
		if len(content) < 5 {
			return nil, offset, fmt.Errorf("mms: float content too short (%d)", len(content))
		}
		data := content[1:]
		if len(data) == 4 {
			bits := binary.BigEndian.Uint32(data[0:4])
			return NewFloat32(math.Float32frombits(bits)), offset, nil
		}
		if len(data) == 8 {
			bits := binary.BigEndian.Uint64(data[0:8])
			return NewFloat64(math.Float64frombits(bits)), offset, nil
		}
		return nil, offset, fmt.Errorf("mms: unsupported float data length %d", len(data))

	case tagMMSBitString:
		if len(content) == 0 {
			return NewBitString(nil, 0), offset, nil
		}
		unusedBits := int(content[0])
		bits := content[1:]
		numBits := 8*len(bits) - unusedBits
		return NewBitString(bits, numBits), offset, nil

	case tagMMSOctetString:
		return NewOctetString(content), offset, nil

	case tagMMSVisibleString:
		return NewVisibleString(string(content)), offset, nil

	case tagMMSString:
		return &Value{typ: TypeString, strVal: string(content)}, offset, nil

	case tagMMSUTCTime:
		t, err := decodeUTCTime(content)
		if err != nil {
			return nil, offset, err
		}
		return NewUTCTime(t), offset, nil

	case tagMMSBinaryTime:
		bt, err := decodeBinaryTime(content)
		if err != nil {
			return nil, offset, err
		}
		return &Value{typ: TypeBinaryTime, binTime: bt}, offset, nil

	case tagMMSArrayCons:
		elements, err := decodeElements(content)
		if err != nil {
			return nil, offset, err
		}
		return NewArray(elements), offset, nil

	case tagMMSStructureCons:
		elements, err := decodeElements(content)
		if err != nil {
			return nil, offset, err
		}
		return NewStructure(elements), offset, nil

	default:
		return nil, offset, fmt.Errorf("mms: unknown data tag 0x%02X", tag)
	}
}

// decodeElements decodes a sequence of values from a content buffer.
func decodeElements(buf []byte) ([]*Value, error) {
	var elements []*Value
	offset := 0
	for offset < len(buf) {
		v, newOff, err := DecodeValue(buf, offset)
		if err != nil {
			return nil, err
		}
		elements = append(elements, v)
		offset = newOff
	}
	return elements, nil
}

// encodeElements encodes a slice of values into a byte slice.
func encodeElements(elements []*Value) ([]byte, error) {
	var body []byte
	for _, elem := range elements {
		enc, err := EncodeValue(elem)
		if err != nil {
			return nil, err
		}
		body = append(body, enc...)
	}
	return body, nil
}

// encodePrimitive encodes a primitive TLV with the given tag and content.
func encodePrimitive(tag byte, content []byte) []byte {
	length := asn1ber.EncodeLength(len(content))
	out := make([]byte, 1+len(length)+len(content))
	out[0] = tag
	copy(out[1:], length)
	copy(out[1+len(length):], content)
	return out
}

// encodeConstructed encodes a constructed TLV with the given already-set tag byte.
func encodeConstructed(tag byte, body []byte) []byte {
	return encodePrimitive(tag, body)
}

// encodeUnsignedContent returns the minimal unsigned integer encoding.
func encodeUnsignedContent(v uint64) []byte {
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

// encodeUTCTime encodes a UTCTime into 8 bytes per IEC 61850-8-1.
func encodeUTCTime(t UTCTime) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint32(buf[0:], t.Seconds)
	buf[4] = byte(t.Fractions >> 16)
	buf[5] = byte(t.Fractions >> 8)
	buf[6] = byte(t.Fractions)
	var quality byte
	if t.LeapSecondsKnown {
		quality |= 0x80
	}
	if t.ClockFailure {
		quality |= 0x40
	}
	if t.ClockNotSynchronized {
		quality |= 0x20
	}
	buf[7] = quality
	return buf
}

// decodeUTCTime decodes an IEC 61850 8-byte UTC timestamp.
func decodeUTCTime(buf []byte) (UTCTime, error) {
	if len(buf) < 8 {
		return UTCTime{}, fmt.Errorf("mms: UTC time too short (%d)", len(buf))
	}
	t := UTCTime{
		Seconds:   binary.BigEndian.Uint32(buf[0:4]),
		Fractions: uint32(buf[4])<<16 | uint32(buf[5])<<8 | uint32(buf[6]),
	}
	q := buf[7]
	t.LeapSecondsKnown = (q & 0x80) != 0
	t.ClockFailure = (q & 0x40) != 0
	t.ClockNotSynchronized = (q & 0x20) != 0
	t.AccuracyClass = q & 0x1F

	return t, nil
}

// encodeBinaryTime encodes a BinaryTime into 4 or 6 bytes.
func encodeBinaryTime(bt BinaryTime) []byte {
	if bt.Size == 6 {
		buf := make([]byte, 6)
		binary.BigEndian.PutUint32(buf[0:], bt.Milliseconds)
		binary.BigEndian.PutUint16(buf[4:], bt.DaysSince1984)
		return buf
	}
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, bt.Milliseconds)
	return buf
}

// decodeBinaryTime decodes a BinaryTime from 4 or 6 bytes.
func decodeBinaryTime(buf []byte) (BinaryTime, error) {
	switch len(buf) {
	case 4:
		return BinaryTime{Milliseconds: binary.BigEndian.Uint32(buf), Size: 4}, nil
	case 6:
		return BinaryTime{
			Milliseconds:  binary.BigEndian.Uint32(buf[0:4]),
			DaysSince1984: binary.BigEndian.Uint16(buf[4:6]),
			Size:          6,
		}, nil
	default:
		return BinaryTime{}, fmt.Errorf("mms: binary time invalid length %d", len(buf))
	}
}
