/*
 *  client_example2.go
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

// server_simple demonstrates a minimal IEC 61850 MMS server.
// It creates a simple data model with one logical device (simpleIO),
// one logical node (GGIO1), and basic I/O data objects.
//
// This is equivalent to examples/server_example_simple in the C library.
//
// Usage:
//
//	./server_simple [port]
//
// Default port: 102 (requires root on Linux for port < 1024; use 10102 for testing)
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

func main() {
	port := 102
	if len(os.Args) > 1 {
		var err error
		port, err = strconv.Atoi(os.Args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid port: %v\n", err)
			os.Exit(1)
		}
	}

	// ---- Build the data model ----
	iedModel, spcsoDAs := buildModel()

	// ---- Create and start the server ----
	iedServer := server.NewIedServer(iedModel, nil)

	if err := iedServer.Start("0.0.0.0", port); err != nil {
		fmt.Printf("Starting server failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Server started on port %d\n", port)

	// ---- Simulate process: toggle outputs every second ----
	go func() {
		tick := 0
		for {
			time.Sleep(1 * time.Second)
			tick++
			// Toggle SPCSO outputs
			for i, da := range spcsoDAs {
				val := mms.NewBoolean((tick+i)%2 == 0)
				iedServer.UpdateAttributeValue(da, val)
			}
		}
	}()

	// ---- Wait for Ctrl+C ----
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Println("Stopping server...")
	iedServer.Stop()
}

// buildModel constructs the IEC 61850 data model.
// Returns the model and a slice of the SPCSO stVal data attributes for simulation.
func buildModel() (*imodel.IedModel, []*imodel.DataAttribute) {
	iedModel := imodel.NewIedModel("simpleIOGenericIO")

	// Logical Device: simpleIO
	ld := imodel.NewLogicalDevice("simpleIO", iedModel)

	// LLN0 (Zero Logical Node) — mandatory in every logical device
	lln0 := imodel.NewLogicalNode("LLN0", ld)
	{
		namPlt := imodel.NewDataObject("NamPlt", lln0)
		da := imodel.NewDataAttribute("vendor", common.FC_DC, common.TypeVisibleStr255, namPlt)
		da.Value = mms.NewVisibleString("libiec61850-Go")
	}

	// GGIO1 (Generic Process I/O logical node)
	ggio1 := imodel.NewLogicalNode("GGIO1", ld)

	// NamPlt
	{
		namPlt := imodel.NewDataObject("NamPlt", ggio1)
		da := imodel.NewDataAttribute("vendor", common.FC_DC, common.TypeVisibleStr255, namPlt)
		da.Value = mms.NewVisibleString("libiec61850-Go")
	}

	// AnIn1: analog input
	{
		anIn1 := imodel.NewDataObject("AnIn1", ggio1)
		magDA := imodel.NewDataAttribute("mag", common.FC_MX, common.TypeFLOAT32, anIn1)
		magDA.Value = mms.NewFloat32(0.0)
		magDA.TriggerOptions = common.TriggerDataChanged

		qDA := imodel.NewDataAttribute("q", common.FC_ST, common.TypeQuality, anIn1)
		qDA.Value = mms.NewBitString([]byte{0x00, 0x00}, 13)

		tDA := imodel.NewDataAttribute("t", common.FC_ST, common.TypeTimestamp, anIn1)
		tDA.Value = mms.NewUTCTime(mms.UTCTimeFromTime(time.Now()))
	}

	// SPCSO1..4: controllable status outputs
	var spcsoDAs []*imodel.DataAttribute
	for i := 1; i <= 4; i++ {
		spcso := imodel.NewDataObject(fmt.Sprintf("SPCSO%d", i), ggio1)

		stVal := imodel.NewDataAttribute("stVal", common.FC_ST, common.TypeBoolean, spcso)
		stVal.Value = mms.NewBoolean(false)
		stVal.TriggerOptions = common.TriggerDataChanged | common.TriggerQualityChanged
		spcsoDAs = append(spcsoDAs, stVal)

		qDA := imodel.NewDataAttribute("q", common.FC_ST, common.TypeQuality, spcso)
		qDA.Value = mms.NewBitString([]byte{0x00, 0x00}, 13)

		tDA := imodel.NewDataAttribute("t", common.FC_ST, common.TypeTimestamp, spcso)
		tDA.Value = mms.NewUTCTime(mms.UTCTimeFromTime(time.Now()))
	}

	// Data set: SPCSO status values
	iedModel.DataSets = append(iedModel.DataSets, &imodel.DataSet{
		Name: "Events",
		Members: []imodel.DataSetMember{
			{Reference: "simpleIO/GGIO1.SPCSO1.stVal", FC: common.FC_ST},
			{Reference: "simpleIO/GGIO1.SPCSO2.stVal", FC: common.FC_ST},
			{Reference: "simpleIO/GGIO1.SPCSO3.stVal", FC: common.FC_ST},
			{Reference: "simpleIO/GGIO1.SPCSO4.stVal", FC: common.FC_ST},
		},
	})

	// Unbuffered Report Control Block
	iedModel.RCBs = append(iedModel.RCBs, &imodel.ReportControlBlock{
		Name:         "EventsRCB01",
		DataSetRef:   "simpleIO/LLN0.Events",
		Buffered:     false,
		RptID:        "Events01",
		ConfRev:      1,
		OptFields:    common.ReportOptSeqNum | common.ReportOptTimeStamp | common.ReportOptReasonForInclusion,
		TrgOps:       common.TriggerDataChanged | common.TriggerQualityChanged | common.TriggerGI,
		IntgPd:       5000,
		MaxInstances: 2,
	})

	return iedModel, spcsoDAs
}
