/*
 *  session_test.go
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

package isosession

import (
	"bytes"
	"testing"
)

func TestBuildParseCNSPDU(t *testing.T) {
	payload := []byte{0x31, 0x01, 0x02, 0x03, 0x04} // mock presentation PDU

	cn := BuildConnectSPDU(payload)
	if cn[0] != SPDUConnect {
		t.Fatalf("CN type: want 0x%02X, got 0x%02X", SPDUConnect, cn[0])
	}
	wantLen := byte(22 + len(payload))
	if cn[1] != wantLen {
		t.Fatalf("CN length: want %d, got %d", wantLen, cn[1])
	}
	// Total SPDU = 2 + 22 + payloadLen
	if len(cn) != 2+22+len(payload) {
		t.Fatalf("CN total bytes: want %d, got %d", 2+22+len(payload), len(cn))
	}

	// Server parses CN
	got, err := ParseConnectSPDU(cn)
	if err != nil {
		t.Fatalf("ParseConnectSPDU: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("ParseConnectSPDU payload mismatch: want %X, got %X", payload, got)
	}
}

func TestBuildParseAcceptSPDU(t *testing.T) {
	payload := []byte{0x31, 0x0A, 0x0B, 0x0C} // mock CPA PDU

	ac := BuildAcceptSPDU(payload)
	if ac[0] != SPDUAccept {
		t.Fatalf("AC type: want 0x%02X, got 0x%02X", SPDUAccept, ac[0])
	}
	// Total = 2 + 18 + payloadLen
	if len(ac) != 2+18+len(payload) {
		t.Fatalf("AC total bytes: want %d, got %d", 2+18+len(payload), len(ac))
	}

	// Client parses AC
	got, err := ParseConnectResponseSPDU(ac)
	if err != nil {
		t.Fatalf("ParseConnectResponseSPDU: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("ParseConnectResponseSPDU payload mismatch: want %X, got %X", payload, got)
	}
}

func TestDataSPDU(t *testing.T) {
	payload := []byte{0xA1, 0x02, 0x03, 0x04, 0x05}

	wrapped := WrapDataSPDU(payload)
	if !bytes.Equal(wrapped[:4], dataSPDUHeader) {
		t.Fatalf("data SPDU header: want %X, got %X", dataSPDUHeader, wrapped[:4])
	}

	got, err := UnwrapDataSPDU(wrapped)
	if err != nil {
		t.Fatalf("UnwrapDataSPDU: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("data SPDU payload mismatch: want %X, got %X", payload, got)
	}
}
