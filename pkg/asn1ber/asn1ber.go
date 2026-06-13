/*
 *  asn1ber.go
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

// Package asn1ber implements Basic Encoding Rules (BER) for ASN.1 as required
// by the MMS (Manufacturing Message Specification, ISO 9506) protocol used in IEC 61850.
//
// BER is a subset of DER (Distinguished Encoding Rules) that is used to encode
// ASN.1 data structures into byte sequences for network transmission.
package asn1ber

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/big"
)

// Tag classes.
const (
	ClassUniversal   = 0x00
	ClassApplication = 0x40
	ClassContext     = 0x80
	ClassPrivate     = 0xC0
)

// Universal tag numbers.
const (
	TagBoolean         = 0x01
	TagInteger         = 0x02
	TagBitString       = 0x03
	TagOctetString     = 0x04
	TagNull            = 0x05
	TagOID             = 0x06
	TagUTF8String      = 0x0C
	TagSequence        = 0x10
	TagSet             = 0x11
	TagNumericString   = 0x12
	TagPrintableString = 0x13
	TagIA5String       = 0x16
	TagUTCTime         = 0x17
	TagGeneralizedTime = 0x18
	TagVisibleString   = 0x1A
	TagGeneralString   = 0x1B
	TagBMPString       = 0x1E
)

// Encoding flags.
const (
	Primitive   = 0x00
	Constructed = 0x20
)

// Tag represents a BER tag with its class, constructed flag, and tag number.
type Tag struct {
	Class       int
	Constructed bool
	Number      int
}

// EncodeTag encodes a BER tag into bytes.
func EncodeTag(class int, constructed bool, number int) []byte {
	b := byte(class)
	if constructed {
		b |= Constructed
	}
	if number < 0x1F {
		return []byte{b | byte(number)}
	}
	// Long form tag
	out := []byte{b | 0x1F}
	// encode number in base-128
	var tmp []byte
	for n := number; n > 0; n >>= 7 {
		tmp = append([]byte{byte(n & 0x7F)}, tmp...)
	}
	for i, t := range tmp {
		if i < len(tmp)-1 {
			t |= 0x80
		}
		out = append(out, t)
	}
	return out
}

// EncodeLength encodes a BER length.
func EncodeLength(length int) []byte {
	if length < 0x80 {
		return []byte{byte(length)}
	}
	if length <= 0xFF {
		return []byte{0x81, byte(length)}
	}
	if length <= 0xFFFF {
		return []byte{0x82, byte(length >> 8), byte(length)}
	}
	if length <= 0xFFFFFF {
		return []byte{0x83, byte(length >> 16), byte(length >> 8), byte(length)}
	}
	return []byte{0x84, byte(length >> 24), byte(length >> 16), byte(length >> 8), byte(length)}
}

// LengthSize returns the number of bytes needed to encode the given length.
func LengthSize(length int) int {
	if length < 0x80 {
		return 1
	}
	if length <= 0xFF {
		return 2
	}
	if length <= 0xFFFF {
		return 3
	}
	if length <= 0xFFFFFF {
		return 4
	}
	return 5
}

// DecodeLength decodes a BER length from buf starting at offset.
// Returns (length, new offset, error).
func DecodeLength(buf []byte, offset int) (int, int, error) {
	if offset >= len(buf) {
		return 0, offset, fmt.Errorf("BER: buffer too short for length at offset %d", offset)
	}
	b := buf[offset]
	offset++
	if b < 0x80 {
		return int(b), offset, nil
	}
	numBytes := int(b & 0x7F)
	if numBytes == 0 {
		return 0, offset, fmt.Errorf("BER: indefinite length not supported")
	}
	if numBytes > 4 {
		return 0, offset, fmt.Errorf("BER: length too large (%d bytes)", numBytes)
	}
	if offset+numBytes > len(buf) {
		return 0, offset, fmt.Errorf("BER: buffer too short for length bytes")
	}
	length := 0
	for i := 0; i < numBytes; i++ {
		length = (length << 8) | int(buf[offset])
		offset++
	}
	return length, offset, nil
}

// DecodeTag decodes a BER tag from buf starting at offset.
// Returns (class, constructed, tagNumber, new offset, error).
func DecodeTag(buf []byte, offset int) (int, bool, int, int, error) {
	if offset >= len(buf) {
		return 0, false, 0, offset, fmt.Errorf("BER: buffer too short for tag")
	}
	b := buf[offset]
	offset++
	class := int(b & 0xC0)
	constructed := (b & Constructed) != 0
	number := int(b & 0x1F)
	if number == 0x1F {
		// Long form tag
		number = 0
		for {
			if offset >= len(buf) {
				return 0, false, 0, offset, fmt.Errorf("BER: buffer too short for long-form tag")
			}
			b = buf[offset]
			offset++
			number = (number << 7) | int(b&0x7F)
			if b&0x80 == 0 {
				break
			}
		}
	}
	return class, constructed, number, offset, nil
}

// TLV (Tag-Length-Value) is the basic building block of BER encoding.
type TLV struct {
	Class       int
	Constructed bool
	Tag         int
	Value       []byte
}

// Encode encodes a TLV into bytes.
func (t *TLV) Encode() []byte {
	tag := EncodeTag(t.Class, t.Constructed, t.Tag)
	length := EncodeLength(len(t.Value))
	out := make([]byte, 0, len(tag)+len(length)+len(t.Value))
	out = append(out, tag...)
	out = append(out, length...)
	out = append(out, t.Value...)
	return out
}

// EncodeTLV is a convenience function to encode a tag-length-value.
func EncodeTLV(class int, constructed bool, tag int, value []byte) []byte {
	t := &TLV{Class: class, Constructed: constructed, Tag: tag, Value: value}
	return t.Encode()
}

// EncodeContextTLV encodes a context-specific TLV.
func EncodeContextTLV(tagNum int, constructed bool, value []byte) []byte {
	return EncodeTLV(ClassContext, constructed, tagNum, value)
}

// EncodeBoolean encodes a BER boolean value.
func EncodeBoolean(v bool) []byte {
	val := byte(0x00)
	if v {
		val = 0xFF
	}
	return EncodeTLV(ClassUniversal, false, TagBoolean, []byte{val})
}

// DecodeBoolean decodes a BER boolean value from content bytes.
func DecodeBoolean(content []byte) (bool, error) {
	if len(content) == 0 {
		return false, fmt.Errorf("BER: empty boolean")
	}
	return content[0] != 0, nil
}

// EncodeInteger encodes a BER integer.
func EncodeInteger(v int64) []byte {
	return EncodeTLV(ClassUniversal, false, TagInteger, encodeIntegerBytes(v))
}

// encodeIntegerBytes returns the minimal two's-complement encoding of v.
func encodeIntegerBytes(v int64) []byte {
	if v == 0 {
		return []byte{0}
	}
	neg := v < 0
	var out []byte
	tmp := v
	for tmp != 0 && tmp != -1 {
		out = append([]byte{byte(tmp)}, out...)
		tmp >>= 8
	}
	if len(out) == 0 {
		// v was exactly -1 or 0 (handled above), shouldn't reach here
		return []byte{byte(v)}
	}
	// Add sign extension byte if needed
	if neg && out[0]&0x80 == 0 {
		out = append([]byte{0xFF}, out...)
	} else if !neg && out[0]&0x80 != 0 {
		out = append([]byte{0x00}, out...)
	}
	return out
}

// DecodeInteger decodes a BER integer from content bytes.
func DecodeInteger(content []byte) (int64, error) {
	if len(content) == 0 {
		return 0, fmt.Errorf("BER: empty integer")
	}
	var v int64
	if content[0]&0x80 != 0 {
		v = -1
	}
	for _, b := range content {
		v = (v << 8) | int64(b)
	}
	return v, nil
}

// DecodeUint32 decodes an unsigned 32-bit integer from content bytes.
func DecodeUint32(content []byte) (uint32, error) {
	if len(content) == 0 || len(content) > 5 {
		return 0, fmt.Errorf("BER: invalid unsigned integer length %d", len(content))
	}
	var v uint64
	for _, b := range content {
		v = (v << 8) | uint64(b)
	}
	return uint32(v), nil
}

// EncodeUnsigned encodes a BER unsigned integer (stored as INTEGER with no sign extension).
func EncodeUnsigned(v uint32) []byte {
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], v)
	// trim leading zeros but keep at least one byte
	start := 0
	for start < 3 && buf[start] == 0 {
		start++
	}
	content := buf[start:]
	// ensure no sign bit is set (add 0x00 prefix if MSB is set)
	if content[0]&0x80 != 0 {
		content = append([]byte{0x00}, content...)
	}
	return EncodeTLV(ClassUniversal, false, TagInteger, content)
}

// EncodeFloat32 encodes a BER float (32-bit) as an MMS FLOAT32.
// MMS uses a 2-byte header: [exponent_width=8, format_width=32] followed by IEEE 754.
func EncodeFloat32(v float32) []byte {
	content := make([]byte, 6)
	content[0] = 8  // exponent width
	content[1] = 32 // format width
	binary.BigEndian.PutUint32(content[2:], math.Float32bits(v))
	return EncodeTLV(ClassUniversal, false, TagOctetString, content)
}

// EncodeFloat64 encodes a BER float (64-bit) as an MMS FLOAT64.
func EncodeFloat64(v float64) []byte {
	content := make([]byte, 10)
	content[0] = 11 // exponent width
	content[1] = 64 // format width
	binary.BigEndian.PutUint64(content[2:], math.Float64bits(v))
	return EncodeTLV(ClassUniversal, false, TagOctetString, content)
}

// DecodeFloat32 decodes an MMS FLOAT32 from content bytes.
func DecodeFloat32(content []byte) (float32, error) {
	if len(content) < 6 {
		return 0, fmt.Errorf("BER: float32 content too short: %d", len(content))
	}
	bits := binary.BigEndian.Uint32(content[2:6])
	return math.Float32frombits(bits), nil
}

// DecodeFloat64 decodes an MMS FLOAT64 from content bytes.
func DecodeFloat64(content []byte) (float64, error) {
	if len(content) < 10 {
		return 0, fmt.Errorf("BER: float64 content too short: %d", len(content))
	}
	bits := binary.BigEndian.Uint64(content[2:10])
	return math.Float64frombits(bits), nil
}

// EncodeOctetString encodes a BER octet string.
func EncodeOctetString(data []byte) []byte {
	return EncodeTLV(ClassUniversal, false, TagOctetString, data)
}

// EncodeVisibleString encodes a BER visible string.
func EncodeVisibleString(s string) []byte {
	return EncodeTLV(ClassUniversal, false, TagVisibleString, []byte(s))
}

// EncodeBitString encodes a BER bit string.
// The first byte of content is the number of unused bits in the last byte.
func EncodeBitString(bits []byte, unusedBits int) []byte {
	content := append([]byte{byte(unusedBits)}, bits...)
	return EncodeTLV(ClassUniversal, false, TagBitString, content)
}

// EncodeOID encodes an ASN.1 Object Identifier.
func EncodeOID(arcs []int) []byte {
	if len(arcs) < 2 {
		return EncodeTLV(ClassUniversal, false, TagOID, []byte{})
	}
	var content []byte
	// First two arcs combined
	content = append(content, byte(arcs[0]*40+arcs[1]))
	for _, arc := range arcs[2:] {
		if arc == 0 {
			content = append(content, 0x00)
			continue
		}
		var tmp []byte
		for arc > 0 {
			tmp = append([]byte{byte(arc & 0x7F)}, tmp...)
			arc >>= 7
		}
		for i, b := range tmp {
			if i < len(tmp)-1 {
				b |= 0x80
			}
			content = append(content, b)
		}
	}
	return EncodeTLV(ClassUniversal, false, TagOID, content)
}

// EncodeSequence encodes a BER SEQUENCE from pre-encoded member bytes.
func EncodeSequence(members ...[]byte) []byte {
	var value []byte
	for _, m := range members {
		value = append(value, m...)
	}
	return EncodeTLV(ClassUniversal, true, TagSequence, value)
}

// ParseTLV reads a single TLV from buf at offset.
// Returns (tlv, new offset, error).
func ParseTLV(buf []byte, offset int) (*TLV, int, error) {
	class, constructed, tagNum, offset, err := DecodeTag(buf, offset)
	if err != nil {
		return nil, offset, err
	}
	length, offset, err := DecodeLength(buf, offset)
	if err != nil {
		return nil, offset, err
	}
	if offset+length > len(buf) {
		return nil, offset, fmt.Errorf("BER: value extends beyond buffer (need %d, have %d)", length, len(buf)-offset)
	}
	value := buf[offset : offset+length]
	offset += length
	return &TLV{Class: class, Constructed: constructed, Tag: tagNum, Value: value}, offset, nil
}

// ParseAll parses all TLVs from buf and returns them.
func ParseAll(buf []byte) ([]*TLV, error) {
	var tlvs []*TLV
	offset := 0
	for offset < len(buf) {
		tlv, newOffset, err := ParseTLV(buf, offset)
		if err != nil {
			return tlvs, err
		}
		tlvs = append(tlvs, tlv)
		offset = newOffset
	}
	return tlvs, nil
}

// EncodeIntegerContent returns just the content bytes for the given integer value,
// without a tag or length wrapper. Useful for constructing composite structures.
func EncodeIntegerContent(v int64) []byte {
	return encodeIntegerBytes(v)
}

// EncodeBigInt encodes a big.Int as a BER integer.
func EncodeBigInt(v *big.Int) []byte {
	b := v.Bytes()
	if v.Sign() < 0 {
		// two's complement
		neg := new(big.Int).Neg(v)
		negBytes := neg.Bytes()
		b = make([]byte, len(negBytes))
		carry := 1
		for i := len(negBytes) - 1; i >= 0; i-- {
			val := int(^negBytes[i]) + carry
			b[i] = byte(val)
			carry = val >> 8
		}
		if b[0]&0x80 == 0 {
			b = append([]byte{0xFF}, b...)
		}
	} else if len(b) > 0 && b[0]&0x80 != 0 {
		b = append([]byte{0x00}, b...)
	}
	if len(b) == 0 {
		b = []byte{0x00}
	}
	return EncodeTLV(ClassUniversal, false, TagInteger, b)
}
