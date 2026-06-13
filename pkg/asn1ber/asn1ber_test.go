/*
 *  asn1ber_test.go
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

package asn1ber

import (
	"testing"
)

func TestEncodeDecodeBoolean(t *testing.T) {
	for _, v := range []bool{true, false} {
		enc := EncodeBoolean(v)
		tlv, _, err := ParseTLV(enc, 0)
		if err != nil {
			t.Fatal(err)
		}
		got, err := DecodeBoolean(tlv.Value)
		if err != nil {
			t.Fatal(err)
		}
		if got != v {
			t.Errorf("boolean round-trip: want %v got %v", v, got)
		}
	}
}

func TestEncodeDecodeInteger(t *testing.T) {
	cases := []int64{0, 1, 127, 128, 255, 256, -1, -128, -129, 32767, -32768, 1<<31 - 1, -(1 << 31)}
	for _, v := range cases {
		enc := EncodeInteger(v)
		tlv, _, err := ParseTLV(enc, 0)
		if err != nil {
			t.Fatalf("encode integer %d: %v", v, err)
		}
		got, err := DecodeInteger(tlv.Value)
		if err != nil {
			t.Fatal(err)
		}
		if got != v {
			t.Errorf("integer round-trip: want %d got %d", v, got)
		}
	}
}

func TestEncodeLength(t *testing.T) {
	cases := []struct {
		length int
		want   []byte
	}{
		{0, []byte{0x00}},
		{127, []byte{0x7F}},
		{128, []byte{0x81, 0x80}},
		{256, []byte{0x82, 0x01, 0x00}},
	}
	for _, c := range cases {
		got := EncodeLength(c.length)
		if len(got) != len(c.want) {
			t.Errorf("length %d: want %v got %v", c.length, c.want, got)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("length %d: want %v got %v", c.length, c.want, got)
			}
		}
	}
}

func TestDecodeLength(t *testing.T) {
	cases := []struct {
		buf  []byte
		want int
	}{
		{[]byte{0x00}, 0},
		{[]byte{0x7F}, 127},
		{[]byte{0x81, 0x80}, 128},
		{[]byte{0x82, 0x01, 0x00}, 256},
	}
	for _, c := range cases {
		got, _, err := DecodeLength(c.buf, 0)
		if err != nil {
			t.Fatalf("decode length %v: %v", c.buf, err)
		}
		if got != c.want {
			t.Errorf("decode length %v: want %d got %d", c.buf, c.want, got)
		}
	}
}

func TestEncodeDecodeVisibleString(t *testing.T) {
	s := "hello world"
	enc := EncodeVisibleString(s)
	tlv, _, err := ParseTLV(enc, 0)
	if err != nil {
		t.Fatal(err)
	}
	if string(tlv.Value) != s {
		t.Errorf("want %q got %q", s, string(tlv.Value))
	}
}

func TestEncodeDecodeFloat32(t *testing.T) {
	v := float32(3.14159)
	enc := EncodeFloat32(v)
	tlv, _, err := ParseTLV(enc, 0)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeFloat32(tlv.Value)
	if err != nil {
		t.Fatal(err)
	}
	if got != v {
		t.Errorf("float32 round-trip: want %f got %f", v, got)
	}
}
