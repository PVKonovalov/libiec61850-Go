/*
 *  server_simple_static.go
 *
 *  Copyright 2013 Michael Zillgith
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

// server_simple_static is a Go port of the C library's server_example_simple.
//
// The data model mirrors sampleModel_with_dataset.cid exactly:
//
//	SampleIED / Device1
//	  LLN0   — Mod, Beh, Health, NamPlt
//	  LPHD1  — PhyNam, PhyHealth, Proxy
//	  DGEN1  — Mod, Beh, Health, NamPlt, OpTmh, GnOpSt, OpTmsRs, TotWh
//	  DSCH1  — Mod, Beh, Health, NamPlt, SchdSt, SchdAbsTm (96 schedule slots)
//	  MMXU1  — Mod, Beh, Health, NamPlt
//	  MMXU2  — Mod, Beh, Health, NamPlt, TotW
//
// Dataset LLN0$dataset1:
//
//	Device1/LLN0$ST$Mod$q
//	Device1/MMXU1$ST$Mod$q
//	Device1/MMXU1$CF$Mod$ctlModel
//
// Usage:
//
//	./server_simple_static [--debug] [port]
//
// Default port: 102
package main

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/PVKonovalov/libiec61850-Go/pkg/iec61850/common"
	imodel "github.com/PVKonovalov/libiec61850-Go/pkg/iec61850/model"
	"github.com/PVKonovalov/libiec61850-Go/pkg/iec61850/server"
	"github.com/PVKonovalov/libiec61850-Go/pkg/mms"
)

const schdAbsTmSlots = 96

func main() {
	port := 102
	debug := false
	var positional []string
	for _, a := range os.Args[1:] {
		if a == "-debug" || a == "--debug" {
			debug = true
		} else {
			positional = append(positional, a)
		}
	}
	if len(positional) > 0 {
		var err error
		port, err = strconv.Atoi(positional[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid port: %v\n", err)
			os.Exit(1)
		}
	}
	if debug {
		mms.SetDebug(true)
	}

	iedModel := buildModel()

	iedServer := server.NewIedServer(iedModel, nil)
	if err := iedServer.Start("0.0.0.0", port); err != nil {
		fmt.Printf("Starting server failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Server started on port %d\n", port)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Println("Stopping server...")
	iedServer.Stop()
}

// buildModel constructs the SampleIED data model, mirroring static_model.c.
func buildModel() *imodel.IedModel {
	now := mms.UTCTimeFromTime(time.Now())

	iedModel := imodel.NewIedModel("SampleIED")
	ld := imodel.NewLogicalDevice("Device1", iedModel)

	buildLLN0(ld, now)
	buildLPHD1(ld, now)
	buildDGEN1(ld, now)
	buildDSCH1(ld, now)
	buildMMXU1(ld, now)
	buildMMXU2(ld, now)

	iedModel.DataSets = append(iedModel.DataSets, &imodel.DataSet{
		Name: "LLN0$dataset1",
		Members: []imodel.DataSetMember{
			{Reference: "Device1/LLN0$ST$Mod$q", FC: common.FC_ST},
			{Reference: "Device1/MMXU1$ST$Mod$q", FC: common.FC_ST},
			{Reference: "Device1/MMXU1$CF$Mod$ctlModel", FC: common.FC_CF},
		},
	})

	return iedModel
}

// addMod adds the standard Mod data object (q/ST, t/ST, ctlModel/CF).
func addMod(ln *imodel.LogicalNode, now mms.UTCTime) {
	mod := imodel.NewDataObject("Mod", ln)
	q := imodel.NewDataAttribute("q", common.FC_ST, common.TypeQuality, mod)
	q.Value = mms.NewBitString([]byte{0x00, 0x00}, 13)
	q.TriggerOptions = common.TriggerQualityChanged
	t := imodel.NewDataAttribute("t", common.FC_ST, common.TypeTimestamp, mod)
	t.Value = mms.NewUTCTime(now)
	ctl := imodel.NewDataAttribute("ctlModel", common.FC_CF, common.TypeINT32, mod)
	ctl.Value = mms.NewInt32(0)
}

// addBeh adds the standard Beh data object (stVal/ST, q/ST, t/ST).
func addBeh(ln *imodel.LogicalNode, now mms.UTCTime) {
	beh := imodel.NewDataObject("Beh", ln)
	stVal := imodel.NewDataAttribute("stVal", common.FC_ST, common.TypeINT32, beh)
	stVal.Value = mms.NewInt32(1) // on
	stVal.TriggerOptions = common.TriggerDataChanged
	q := imodel.NewDataAttribute("q", common.FC_ST, common.TypeQuality, beh)
	q.Value = mms.NewBitString([]byte{0x00, 0x00}, 13)
	q.TriggerOptions = common.TriggerQualityChanged
	t := imodel.NewDataAttribute("t", common.FC_ST, common.TypeTimestamp, beh)
	t.Value = mms.NewUTCTime(now)
}

// addHealth adds the standard Health data object (stVal/ST, q/ST, t/ST).
func addHealth(ln *imodel.LogicalNode, now mms.UTCTime) {
	health := imodel.NewDataObject("Health", ln)
	stVal := imodel.NewDataAttribute("stVal", common.FC_ST, common.TypeINT32, health)
	stVal.Value = mms.NewInt32(1) // ok
	stVal.TriggerOptions = common.TriggerDataChanged
	q := imodel.NewDataAttribute("q", common.FC_ST, common.TypeQuality, health)
	q.Value = mms.NewBitString([]byte{0x00, 0x00}, 13)
	q.TriggerOptions = common.TriggerQualityChanged
	t := imodel.NewDataAttribute("t", common.FC_ST, common.TypeTimestamp, health)
	t.Value = mms.NewUTCTime(now)
}

// addNamPlt adds a NamPlt data object with vendor, swRev, d (all DC).
func addNamPlt(ln *imodel.LogicalNode, vendor, swRev, d string) {
	namPlt := imodel.NewDataObject("NamPlt", ln)
	v := imodel.NewDataAttribute("vendor", common.FC_DC, common.TypeVisibleStr255, namPlt)
	v.Value = mms.NewVisibleString(vendor)
	sw := imodel.NewDataAttribute("swRev", common.FC_DC, common.TypeVisibleStr255, namPlt)
	sw.Value = mms.NewVisibleString(swRev)
	desc := imodel.NewDataAttribute("d", common.FC_DC, common.TypeVisibleStr255, namPlt)
	desc.Value = mms.NewVisibleString(d)
}

// buildLLN0 builds the zero logical node.
// LLN0: Mod, Beh, Health, NamPlt(vendor,swRev,d,configRev,ldNs)
func buildLLN0(ld *imodel.LogicalDevice, now mms.UTCTime) {
	ln := imodel.NewLogicalNode("LLN0", ld)
	addMod(ln, now)
	addBeh(ln, now)
	addHealth(ln, now)

	namPlt := imodel.NewDataObject("NamPlt", ln)
	vendor := imodel.NewDataAttribute("vendor", common.FC_DC, common.TypeVisibleStr255, namPlt)
	vendor.Value = mms.NewVisibleString("libiec61850-Go")
	swRev := imodel.NewDataAttribute("swRev", common.FC_DC, common.TypeVisibleStr255, namPlt)
	swRev.Value = mms.NewVisibleString("1.0.0")
	d := imodel.NewDataAttribute("d", common.FC_DC, common.TypeVisibleStr255, namPlt)
	d.Value = mms.NewVisibleString("SampleIED LLN0")
	configRev := imodel.NewDataAttribute("configRev", common.FC_DC, common.TypeVisibleStr255, namPlt)
	configRev.Value = mms.NewVisibleString("1")
	ldNs := imodel.NewDataAttribute("ldNs", common.FC_EX, common.TypeVisibleStr255, namPlt)
	ldNs.Value = mms.NewVisibleString("IEC 61850-7-4:2007B4")
}

// buildLPHD1 builds the physical device logical node.
// LPHD1: PhyNam(vendor/DC), PhyHealth(stVal,q,t), Proxy(stVal,q,t)
func buildLPHD1(ld *imodel.LogicalDevice, now mms.UTCTime) {
	ln := imodel.NewLogicalNode("LPHD1", ld)

	phyNam := imodel.NewDataObject("PhyNam", ln)
	phyVendor := imodel.NewDataAttribute("vendor", common.FC_DC, common.TypeVisibleStr255, phyNam)
	phyVendor.Value = mms.NewVisibleString("libiec61850-Go")

	phyHealth := imodel.NewDataObject("PhyHealth", ln)
	phStVal := imodel.NewDataAttribute("stVal", common.FC_ST, common.TypeINT32, phyHealth)
	phStVal.Value = mms.NewInt32(1)
	phStVal.TriggerOptions = common.TriggerDataChanged
	phQ := imodel.NewDataAttribute("q", common.FC_ST, common.TypeQuality, phyHealth)
	phQ.Value = mms.NewBitString([]byte{0x00, 0x00}, 13)
	phQ.TriggerOptions = common.TriggerQualityChanged
	phT := imodel.NewDataAttribute("t", common.FC_ST, common.TypeTimestamp, phyHealth)
	phT.Value = mms.NewUTCTime(now)

	proxy := imodel.NewDataObject("Proxy", ln)
	prStVal := imodel.NewDataAttribute("stVal", common.FC_ST, common.TypeBoolean, proxy)
	prStVal.Value = mms.NewBoolean(false)
	prStVal.TriggerOptions = common.TriggerDataChanged
	prQ := imodel.NewDataAttribute("q", common.FC_ST, common.TypeQuality, proxy)
	prQ.Value = mms.NewBitString([]byte{0x00, 0x00}, 13)
	prQ.TriggerOptions = common.TriggerQualityChanged
	prT := imodel.NewDataAttribute("t", common.FC_ST, common.TypeTimestamp, proxy)
	prT.Value = mms.NewUTCTime(now)
}

// addStQT adds stVal(INT32)/ST, q/ST, t/ST to a DataObject.
func addStQT(do *imodel.DataObject, now mms.UTCTime) {
	stVal := imodel.NewDataAttribute("stVal", common.FC_ST, common.TypeINT32, do)
	stVal.Value = mms.NewInt32(0)
	stVal.TriggerOptions = common.TriggerDataChanged
	q := imodel.NewDataAttribute("q", common.FC_ST, common.TypeQuality, do)
	q.Value = mms.NewBitString([]byte{0x00, 0x00}, 13)
	q.TriggerOptions = common.TriggerQualityChanged
	t := imodel.NewDataAttribute("t", common.FC_ST, common.TypeTimestamp, do)
	t.Value = mms.NewUTCTime(now)
}

// buildDGEN1 builds the DER unit generator logical node.
// DGEN1: Mod, Beh, Health, NamPlt, OpTmh, GnOpSt, OpTmsRs, TotWh
func buildDGEN1(ld *imodel.LogicalDevice, now mms.UTCTime) {
	ln := imodel.NewLogicalNode("DGEN1", ld)
	addMod(ln, now)
	addBeh(ln, now)
	addHealth(ln, now)
	addNamPlt(ln, "libiec61850-Go", "1.0.0", "DER Generator")

	opTmh := imodel.NewDataObject("OpTmh", ln)
	addStQT(opTmh, now)

	gnOpSt := imodel.NewDataObject("GnOpSt", ln)
	addStQT(gnOpSt, now)

	opTmsRs := imodel.NewDataObject("OpTmsRs", ln)
	addStQT(opTmsRs, now)

	// TotWh: mag (AnalogueValue represented as FLOAT32), q, t  — FC_MX
	totWh := imodel.NewDataObject("TotWh", ln)
	mag := imodel.NewDataAttribute("mag", common.FC_MX, common.TypeFLOAT32, totWh)
	mag.Value = mms.NewFloat32(0.0)
	mag.TriggerOptions = common.TriggerDataChanged
	magQ := imodel.NewDataAttribute("q", common.FC_MX, common.TypeQuality, totWh)
	magQ.Value = mms.NewBitString([]byte{0x00, 0x00}, 13)
	magQ.TriggerOptions = common.TriggerQualityChanged
	magT := imodel.NewDataAttribute("t", common.FC_MX, common.TypeTimestamp, totWh)
	magT.Value = mms.NewUTCTime(now)
}

// buildDSCH1 builds the DER schedule logical node.
// DSCH1: Mod, Beh, Health, NamPlt, SchdSt, SchdAbsTm (96 slots: val/SP/FLOAT32 + time/SP/TIMESTAMP)
func buildDSCH1(ld *imodel.LogicalDevice, now mms.UTCTime) {
	ln := imodel.NewLogicalNode("DSCH1", ld)
	addMod(ln, now)
	addBeh(ln, now)
	addHealth(ln, now)
	addNamPlt(ln, "libiec61850-Go", "1.0.0", "DER Schedule")

	schdSt := imodel.NewDataObject("SchdSt", ln)
	addStQT(schdSt, now)

	// SchdAbsTm: array of 96 schedule slots.
	// Each slot has val (SP, FLOAT32) and time (SP, TIMESTAMP).
	// The model represents all slots as children of the SchdAbsTm DataObject.
	schdAbsTm := imodel.NewDataObject("SchdAbsTm", ln)
	for i := 0; i < schdAbsTmSlots; i++ {
		v := imodel.NewDataAttribute(fmt.Sprintf("val_%d", i), common.FC_SP, common.TypeFLOAT32, schdAbsTm)
		v.Value = mms.NewFloat32(0.0)
		tm := imodel.NewDataAttribute(fmt.Sprintf("time_%d", i), common.FC_SP, common.TypeTimestamp, schdAbsTm)
		tm.Value = mms.NewUTCTime(now)
	}
}

// buildMMXU1 builds the first measurement logical node (no measurement DOs in this model).
// MMXU1: Mod, Beh, Health, NamPlt
func buildMMXU1(ld *imodel.LogicalDevice, now mms.UTCTime) {
	ln := imodel.NewLogicalNode("MMXU1", ld)
	addMod(ln, now)
	addBeh(ln, now)
	addHealth(ln, now)
	addNamPlt(ln, "libiec61850-Go", "1.0.0", "Measurement Unit 1")
}

// buildMMXU2 builds the second measurement logical node.
// MMXU2: Mod, Beh, Health, NamPlt, TotW
func buildMMXU2(ld *imodel.LogicalDevice, now mms.UTCTime) {
	ln := imodel.NewLogicalNode("MMXU2", ld)
	addMod(ln, now)
	addBeh(ln, now)
	addHealth(ln, now)
	addNamPlt(ln, "libiec61850-Go", "1.0.0", "Measurement Unit 2")

	// TotW: mag (AnalogueValue as FLOAT32), q, t — FC_MX
	totW := imodel.NewDataObject("TotW", ln)
	mag := imodel.NewDataAttribute("mag", common.FC_MX, common.TypeFLOAT32, totW)
	mag.Value = mms.NewFloat32(0.0)
	mag.TriggerOptions = common.TriggerDataChanged
	magQ := imodel.NewDataAttribute("q", common.FC_MX, common.TypeQuality, totW)
	magQ.Value = mms.NewBitString([]byte{0x00, 0x00}, 13)
	magQ.TriggerOptions = common.TriggerQualityChanged
	magT := imodel.NewDataAttribute("t", common.FC_MX, common.TypeTimestamp, totW)
	magT.Value = mms.NewUTCTime(now)
}
