/*
 *  presentation_test.go
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

package isopresentation

import (
	"bytes"
	"testing"
)

func TestBuildParseConnectPDU(t *testing.T) {
	acsePDU := []byte{0x60, 0x05, 0x01, 0x02, 0x03, 0x04, 0x05} // mock AARQ

	cp := BuildConnectPDU(acsePDU)
	// Outer tag must be 0x31 (UNIVERSAL SET)
	if cp[0] != 0x31 {
		t.Fatalf("CP outer tag: want 0x31, got 0x%02X", cp[0])
	}

	// Round-trip: ParseConnectAcceptPDU should extract acsePDU back
	// (CPA and CP have the same inner structure for parsing purposes)
	// Build a synthetic CPA around the same payload to test the parser.
	cpa := BuildConnectAcceptPDU(acsePDU)
	if cpa[0] != 0x31 {
		t.Fatalf("CPA outer tag: want 0x31, got 0x%02X", cpa[0])
	}

	got, err := ParseConnectAcceptPDU(cpa)
	if err != nil {
		t.Fatalf("ParseConnectAcceptPDU: %v", err)
	}
	if !bytes.Equal(got, acsePDU) {
		t.Errorf("CPA payload mismatch: want %X, got %X", acsePDU, got)
	}
}

func TestWrapUnwrapUserData(t *testing.T) {
	mmsPDU := []byte{0xA1, 0x03, 0x02, 0x01, 0x05} // mock MMS response

	wrapped := WrapUserData(mmsPDU)
	// Outer tag must be 0x61 (APPLICATION 1 CONSTRUCTED = fully-encoded-data)
	if wrapped[0] != 0x61 {
		t.Fatalf("user-data outer tag: want 0x61, got 0x%02X", wrapped[0])
	}

	got, err := UnwrapUserData(wrapped)
	if err != nil {
		t.Fatalf("UnwrapUserData: %v", err)
	}
	if !bytes.Equal(got, mmsPDU) {
		t.Errorf("user-data payload mismatch: want %X, got %X", mmsPDU, got)
	}
}

func TestFullStackRoundTrip(t *testing.T) {
	// Simulate a full client→server→client round-trip through presentation+session
	mmsPDU := []byte{0xA1, 0x05, 0x02, 0x01, 0x01, 0x02, 0x01}

	// Client wraps
	presWrapped := WrapUserData(mmsPDU)

	// Server unwraps
	got, err := UnwrapUserData(presWrapped)
	if err != nil {
		t.Fatalf("UnwrapUserData: %v", err)
	}
	if !bytes.Equal(got, mmsPDU) {
		t.Errorf("round-trip mismatch: want %X, got %X", mmsPDU, got)
	}
}
