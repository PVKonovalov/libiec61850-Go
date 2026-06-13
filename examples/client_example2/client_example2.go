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

// client_example2 demonstrates basic IEC 61850 MMS client operations:
// reading a data attribute, writing a value, reading a data set, and
// subscribing to a report control block.
//
// This example is equivalent to examples/iec61850_client_example1 in
// the original libiec61850 C library.
//
// Usage:
//
//	./client_example1 [host] [port]
//
// Default host: localhost, Default port: 102
package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/PVKonovalov/libiec61850-Go/pkg/iec61850/client"
	"github.com/PVKonovalov/libiec61850-Go/pkg/iec61850/common"
	"github.com/PVKonovalov/libiec61850-Go/pkg/mms"
)

func main() {
	host := "localhost"
	port := 102

	if len(os.Args) > 1 {
		host = os.Args[1]
	}
	if len(os.Args) > 2 {
		var err error
		port, err = strconv.Atoi(os.Args[2])
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid port: %v\n", err)
			os.Exit(1)
		}
	}

	address := fmt.Sprintf("%s:%d", host, port)
	fmt.Printf("Connecting to %s\n", address)

	conn, err := client.Dial(address)
	if err != nil {
		fmt.Printf("Failed to connect to %s: %v\n", address, err)
		return
	}
	defer conn.Close()

	fmt.Println("Connected")

	// ---- Read an analog measurement value ----
	value, err := conn.ReadObject(
		"STRATON_IEDLDevice/MMXU1.TotW.mag.f",
		common.FC_MX,
	)
	if err != nil {
		fmt.Printf("failed to read analog value: %v\n", err)
	} else {
		switch value.Type() {
		case mms.TypeFloat:
			fmt.Printf("read float value: %f\n", value.GetFloat32())
		case mms.TypeDataAccessError:
			fmt.Printf("Failed to read value (error code: %d)\n", value.GetDataAccessError())
		default:
			fmt.Printf("read value: %s\n", value)
		}
	}

	// ---- Write a visible string to the server ----
	writeVal := mms.NewVisibleString("libiec61850-Go")
	err = conn.WriteObject(
		"STRATON_IEDLDevice/DPMC1.NamPlt.vendor",
		common.FC_DC,
		writeVal,
	)
	if err != nil {
		fmt.Printf("failed to write STRATON_IEDLDevice/DPMC1.NamPlt.vendor: %v\n", err)
	} else {
		fmt.Printf("written value: %s\n", writeVal)
	}

	// ---- Read a data set ----
	dataSet, err := conn.ReadDataSetValues("STRATON_IEDLDevice/MMXU1$DSMMXU", nil)
	if err != nil {
		fmt.Printf("failed to read dataset: %v\n", err)
		goto close_connection
	}
	fmt.Printf("dataset values: %s\n", dataSet.GetDataSetValues())

	// ---- Subscribe to reports ----
	{
		rcb, err := conn.GetRCBValues("STRATON_IEDLDevice/MMXU1$RP$urcbMX02")
		if err != nil {
			fmt.Printf("failed to get RCB values: %v\n", err)
			goto close_connection
		}

		fmt.Printf("RptEna = %v\n", rcb.RptEna)

		// Install report handler
		conn.InstallReportHandler(
			"STRATON_IEDLDevice/MMXU1$RP$urcbMX02",
			rcb.RptID,
			reportCallbackFunction,
		)

		// Configure and enable reporting
		rcb.TrgOps = common.TriggerDataChanged | common.TriggerDataUpdate | common.TriggerIntegrity | common.TriggerGI
		rcb.RptEna = true
		rcb.IntgPd = 0
		//rcb.DataSetRef = "STRATON_IEDLDevice/MMXU1$DSMMXU"

		err = conn.SetRCBValues(rcb,
			client.RCBElementRptEna|client.RCBElementTrgOps|client.RCBElementIntgPd,
			true,
		)
		if err != nil {
			fmt.Printf("report activation failed: %v\n", err)
		} else {
			fmt.Printf("report activated\n")
		}

		time.Sleep(1 * time.Second)

		// Trigger a GI (General Interrogation) report
		rcb.GI = true
		err = conn.SetRCBValues(rcb, client.RCBElementGI, true)
		if err != nil {
			fmt.Printf("error triggering GI report: %v\n", err)
		}

		time.Sleep(60 * time.Second)

		// Disable reporting
		rcb.RptEna = false
		err = conn.SetRCBValues(rcb, client.RCBElementRptEna, true)
		if err != nil {
			fmt.Printf("disable reporting failed: %v\n", err)
		}
	}

close_connection:
	conn.Close()
}

// reportCallbackFunction handles received reports.
func reportCallbackFunction(report *client.Report) {
	fmt.Printf("received report for %s\n", report.RCBReference)

	if report.DataSetValues == nil {
		return
	}
	for i := 0; i < report.DataSetValues.Size(); i++ {
		reason := common.ReasonNotIncluded
		if i < len(report.ReasonForInclusion) {
			reason = report.ReasonForInclusion[i]
		}
		if reason == common.ReasonNotIncluded {
			continue
		}
		elem := report.DataSetValues.GetElement(i)
		if elem == nil {
			continue
		}

		value, quality, ts := extractMeasurement(elem)
		fmt.Printf("  Object: DataPoint_%d, Value: %f, Quality: %d, Timestamp: %s\n",
			i, value, quality, ts.Format("2006-01-02 15:04:05.000 -0700 MST"))
	}
}

// extractMeasurement pulls (mag.f, quality, timestamp) from an IEC 61850 analogue
// measurement STRUCTURE of the form { mag{f:FLOAT}, q:BIT-STRING, t:UTC-TIME }.
func extractMeasurement(elem *mms.Value) (value float64, quality uint16, ts time.Time) {
	if elem == nil || elem.Type() != mms.TypeStructure || elem.Size() < 3 {
		return
	}

	// [0] mag — nested STRUCTURE with a single FLOAT member
	if mag := elem.GetElement(0); mag != nil {
		switch mag.Type() {
		case mms.TypeStructure:
			if mag.Size() > 0 {
				if f := mag.GetElement(0); f != nil && f.Type() == mms.TypeFloat {
					value = f.GetFloat64()
				}
			}
		case mms.TypeFloat:
			value = mag.GetFloat64()
		}
	}

	// [1] q — quality as big-endian uint16 from the BitString bytes
	if q := elem.GetElement(1); q != nil && q.Type() == mms.TypeBitString {
		bits, _ := q.GetBitString()
		if len(bits) >= 2 {
			quality = uint16(bits[0])<<8 | uint16(bits[1])
		} else if len(bits) == 1 {
			quality = uint16(bits[0])
		}
	}

	// [2] t — UTC timestamp converted to local time
	if t := elem.GetElement(2); t != nil && t.Type() == mms.TypeUTCTime {
		ts = t.GetUTCTime().ToTime().Local()
	}
	return
}
