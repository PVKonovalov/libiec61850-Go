/*
 *  common_test.go
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

package common

import (
	"testing"
)

func TestFunctionalConstraintString(t *testing.T) {
	cases := []struct {
		fc   FunctionalConstraint
		want string
	}{
		{FC_ST, "ST"},
		{FC_MX, "MX"},
		{FC_SP, "SP"},
		{FC_CF, "CF"},
		{FC_DC, "DC"},
		{FC_CO, "CO"},
		{FC_BR, "BR"},
		{FC_RP, "RP"},
		{FC_GO, "GO"},
		{FC_ALL, "ALL"},
	}
	for _, c := range cases {
		got := c.fc.String()
		if got != c.want {
			t.Errorf("FC %d: want %q got %q", int(c.fc), c.want, got)
		}
	}
}

func TestParseFC(t *testing.T) {
	cases := []struct {
		s    string
		want FunctionalConstraint
	}{
		{"ST", FC_ST},
		{"MX", FC_MX},
		{"mx", FC_MX}, // case-insensitive
		{"BR", FC_BR},
		{"*", FC_ALL},
		{"ALL", FC_ALL},
		{"XX", FC_NONE},
	}
	for _, c := range cases {
		got := ParseFC(c.s)
		if got != c.want {
			t.Errorf("ParseFC(%q): want %d got %d", c.s, int(c.want), int(got))
		}
	}
}

func TestParseObjectReference(t *testing.T) {
	cases := []struct {
		ref  string
		want ObjectReference
	}{
		{
			"simpleIO/GGIO1.AnIn1.mag.f$MX",
			ObjectReference{
				LogicalDevice: "simpleIO",
				LogicalNode:   "GGIO1",
				DataObject:    "AnIn1",
				DataAttribute: "mag.f",
				FC:            FC_MX,
			},
		},
		{
			"LD1/LLN0.NamPlt",
			ObjectReference{
				LogicalDevice: "LD1",
				LogicalNode:   "LLN0",
				DataObject:    "NamPlt",
				FC:            FC_NONE,
			},
		},
	}
	for _, c := range cases {
		got, err := ParseObjectReference(c.ref)
		if err != nil {
			t.Errorf("ParseObjectReference(%q): %v", c.ref, err)
			continue
		}
		if got.LogicalDevice != c.want.LogicalDevice {
			t.Errorf("LD: want %q got %q", c.want.LogicalDevice, got.LogicalDevice)
		}
		if got.LogicalNode != c.want.LogicalNode {
			t.Errorf("LN: want %q got %q", c.want.LogicalNode, got.LogicalNode)
		}
		if got.DataObject != c.want.DataObject {
			t.Errorf("DO: want %q got %q", c.want.DataObject, got.DataObject)
		}
		if got.FC != c.want.FC {
			t.Errorf("FC: want %d got %d", int(c.want.FC), int(got.FC))
		}
	}
}

func TestQuality(t *testing.T) {
	q := Quality(0x0000)
	if !q.IsGood() {
		t.Error("zero quality should be GOOD")
	}
	if q.IsInvalid() {
		t.Error("zero quality should not be INVALID")
	}

	q = QualityInvalid
	if !q.IsInvalid() {
		t.Error("invalid quality should be INVALID")
	}

	q = QualityTest | QualitySource
	if !q.IsTest() {
		t.Error("should be test")
	}
	if !q.IsSubstituted() {
		t.Error("should be substituted")
	}
}

func TestMMSObjectReference(t *testing.T) {
	or := ObjectReference{
		LogicalDevice: "simpleIO",
		LogicalNode:   "GGIO1",
		DataObject:    "AnIn1",
		DataAttribute: "mag",
		FC:            FC_MX,
	}
	domainID, itemID := or.MMSObjectReference()
	if domainID != "simpleIO" {
		t.Errorf("domainID: want simpleIO got %s", domainID)
	}
	// Expected: GGIO1$MX$AnIn1$mag
	expectedItem := "GGIO1$MX$AnIn1$mag"
	if itemID != expectedItem {
		t.Errorf("itemID: want %q got %q", expectedItem, itemID)
	}
}
