/*
 *  encode_test.go
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
	"math"
	"testing"
)

func TestEncodeDecodeBoolean(t *testing.T) {
	for _, v := range []bool{true, false} {
		val := NewBoolean(v)
		enc, err := EncodeValue(val)
		if err != nil {
			t.Fatalf("encode boolean %v: %v", v, err)
		}
		decoded, _, err := DecodeValue(enc, 0)
		if err != nil {
			t.Fatalf("decode boolean %v: %v", v, err)
		}
		if decoded.Type() != TypeBoolean {
			t.Errorf("type: want BOOLEAN, got %s", decoded.Type())
		}
		if decoded.GetBoolean() != v {
			t.Errorf("boolean round-trip: want %v got %v", v, decoded.GetBoolean())
		}
	}
}

func TestEncodeDecodeIntegers(t *testing.T) {
	cases := []int64{0, 1, 127, 128, 255, -1, -128, -129, 32767, -32768, 1<<31 - 1, -(1 << 31)}
	for _, v := range cases {
		val := NewInt64(v)
		enc, err := EncodeValue(val)
		if err != nil {
			t.Fatalf("encode int64 %d: %v", v, err)
		}
		decoded, _, err := DecodeValue(enc, 0)
		if err != nil {
			t.Fatalf("decode int64 %d: %v", v, err)
		}
		if decoded.GetInt64() != v {
			t.Errorf("int64 round-trip: want %d got %d", v, decoded.GetInt64())
		}
	}
}

func TestEncodeDecodeUnsigned(t *testing.T) {
	cases := []uint32{0, 1, 127, 128, 255, 256, 65535, 1<<32 - 1}
	for _, v := range cases {
		val := NewUint32(v)
		enc, err := EncodeValue(val)
		if err != nil {
			t.Fatalf("encode uint32 %d: %v", v, err)
		}
		decoded, _, err := DecodeValue(enc, 0)
		if err != nil {
			t.Fatalf("decode uint32 %d: %v", v, err)
		}
		if decoded.GetUint32() != v {
			t.Errorf("uint32 round-trip: want %d got %d", v, decoded.GetUint32())
		}
	}
}

func TestEncodeDecodeFloat32(t *testing.T) {
	cases := []float32{0, 1.0, -1.0, 3.14159, math.MaxFloat32, -math.MaxFloat32, 1e-10}
	for _, v := range cases {
		val := NewFloat32(v)
		enc, err := EncodeValue(val)
		if err != nil {
			t.Fatalf("encode float32 %f: %v", v, err)
		}
		decoded, _, err := DecodeValue(enc, 0)
		if err != nil {
			t.Fatalf("decode float32 %f: %v", v, err)
		}
		if decoded.GetFloat32() != v {
			t.Errorf("float32 round-trip: want %f got %f", v, decoded.GetFloat32())
		}
	}
}

func TestEncodeDecodeVisibleString(t *testing.T) {
	cases := []string{"", "hello", "libiec61850.com", "test string with spaces"}
	for _, s := range cases {
		val := NewVisibleString(s)
		enc, err := EncodeValue(val)
		if err != nil {
			t.Fatalf("encode string %q: %v", s, err)
		}
		decoded, _, err := DecodeValue(enc, 0)
		if err != nil {
			t.Fatalf("decode string %q: %v", s, err)
		}
		if decoded.GetVisibleString() != s {
			t.Errorf("string round-trip: want %q got %q", s, decoded.GetVisibleString())
		}
	}
}

func TestEncodeDecodeOctetString(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03, 0xFF, 0x00}
	val := NewOctetString(data)
	enc, err := EncodeValue(val)
	if err != nil {
		t.Fatalf("encode octet string: %v", err)
	}
	decoded, _, err := DecodeValue(enc, 0)
	if err != nil {
		t.Fatalf("decode octet string: %v", err)
	}
	got := decoded.GetOctetString()
	if len(got) != len(data) {
		t.Fatalf("octet string length: want %d got %d", len(data), len(got))
	}
	for i := range data {
		if got[i] != data[i] {
			t.Errorf("octet string[%d]: want 0x%02X got 0x%02X", i, data[i], got[i])
		}
	}
}

func TestEncodeDecodeBitString(t *testing.T) {
	bits := []byte{0xAB, 0xCD}
	numBits := 13
	val := NewBitString(bits, numBits)
	enc, err := EncodeValue(val)
	if err != nil {
		t.Fatalf("encode bit string: %v", err)
	}
	decoded, _, err := DecodeValue(enc, 0)
	if err != nil {
		t.Fatalf("decode bit string: %v", err)
	}
	_, gotBits := decoded.GetBitString()
	if gotBits != numBits {
		t.Errorf("bit count: want %d got %d", numBits, gotBits)
	}
}

func TestEncodeDecodeUTCTime(t *testing.T) {
	ts := UTCTimeFromTime(UTCTime{Seconds: 1700000000, Fractions: 0}.ToTime())
	val := NewUTCTime(ts)
	enc, err := EncodeValue(val)
	if err != nil {
		t.Fatalf("encode UTC time: %v", err)
	}
	decoded, _, err := DecodeValue(enc, 0)
	if err != nil {
		t.Fatalf("decode UTC time: %v", err)
	}
	got := decoded.GetUTCTime()
	if got.Seconds != ts.Seconds {
		t.Errorf("UTC seconds: want %d got %d", ts.Seconds, got.Seconds)
	}
}

func TestEncodeDecodeStructure(t *testing.T) {
	members := []*Value{
		NewBoolean(true),
		NewInt32(42),
		NewVisibleString("hello"),
		NewFloat32(1.5),
	}
	val := NewStructure(members)
	enc, err := EncodeValue(val)
	if err != nil {
		t.Fatalf("encode structure: %v", err)
	}
	decoded, _, err := DecodeValue(enc, 0)
	if err != nil {
		t.Fatalf("decode structure: %v", err)
	}
	if decoded.Type() != TypeStructure {
		t.Fatalf("type: want STRUCTURE, got %s", decoded.Type())
	}
	if decoded.Size() != len(members) {
		t.Fatalf("size: want %d got %d", len(members), decoded.Size())
	}
	if decoded.GetElement(0).GetBoolean() != true {
		t.Error("element 0: expected true")
	}
	if decoded.GetElement(1).GetInt32() != 42 {
		t.Error("element 1: expected 42")
	}
	if decoded.GetElement(2).GetVisibleString() != "hello" {
		t.Error("element 2: expected 'hello'")
	}
}

func TestEncodeDecodeArray(t *testing.T) {
	elements := []*Value{
		NewInt32(1),
		NewInt32(2),
		NewInt32(3),
	}
	val := NewArray(elements)
	enc, err := EncodeValue(val)
	if err != nil {
		t.Fatalf("encode array: %v", err)
	}
	decoded, _, err := DecodeValue(enc, 0)
	if err != nil {
		t.Fatalf("decode array: %v", err)
	}
	if decoded.Type() != TypeArray {
		t.Fatalf("type: want ARRAY, got %s", decoded.Type())
	}
	if decoded.Size() != 3 {
		t.Fatalf("size: want 3 got %d", decoded.Size())
	}
	for i := 0; i < 3; i++ {
		if decoded.GetElement(i).GetInt32() != int32(i+1) {
			t.Errorf("element %d: expected %d", i, i+1)
		}
	}
}

func TestValueClone(t *testing.T) {
	original := NewStructure([]*Value{
		NewBoolean(true),
		NewFloat32(3.14),
		NewVisibleString("test"),
	})
	clone := original.Clone()

	if clone.Size() != original.Size() {
		t.Errorf("clone size mismatch: %d vs %d", clone.Size(), original.Size())
	}
	if clone.GetElement(0).GetBoolean() != true {
		t.Error("clone element 0 mismatch")
	}
	if clone.GetElement(2).GetVisibleString() != "test" {
		t.Error("clone string mismatch")
	}
}
