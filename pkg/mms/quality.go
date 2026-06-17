package mms

import (
	"math/bits"

	"github.com/PVKonovalov/libiec61850-Go/pkg/iec61850/common"
)

// NewQuality creates a 13-bit BIT_STRING Value from a common.Quality.
//
// IEC 61850 Quality is encoded as a 13-bit BIT STRING where integer bit N
// maps to byte[N/8] bit (7 - N%8) — i.e. the bits within each byte are
// reversed compared to the integer representation. This matches the C library's
// MmsValue_setBitStringFromInteger / Quality_toMmsValue behaviour.
func NewQuality(q common.Quality) *Value {
	v := uint16(q)
	return NewBitString([]byte{bits.Reverse8(byte(v)), bits.Reverse8(byte(v >> 8))}, 13)
}

// QualityFromValue decodes a 13-bit BIT_STRING Value back into a common.Quality.
// Returns QualityGood if v is nil or not a BIT_STRING.
func QualityFromValue(v *Value) common.Quality {
	if v == nil || v.typ != TypeBitString {
		return common.QualityGood
	}
	b, _ := v.GetBitString()
	if len(b) == 0 {
		return common.QualityGood
	}
	b0 := bits.Reverse8(b[0])
	var b1 byte
	if len(b) > 1 {
		b1 = bits.Reverse8(b[1])
	}
	return common.Quality(uint16(b0) | uint16(b1)<<8)
}
