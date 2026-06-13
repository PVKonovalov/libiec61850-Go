/*
 *  main.go
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

// goose_publisher demonstrates publishing GOOSE messages over raw Ethernet.
//
// GOOSE requires a raw Ethernet socket. On Linux this requires root or
// the CAP_NET_RAW capability:
//
//	sudo ./goose_publisher eth0
//
// Usage:
//
//	./goose_publisher [interface]
//
// Default interface: eth0
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/PVKonovalov/libiec61850-Go/pkg/goose"
	"github.com/PVKonovalov/libiec61850-Go/pkg/iec61850/common"
	"github.com/PVKonovalov/libiec61850-Go/pkg/mms"
)

func main() {
	ifaceName := "eth0"
	if len(os.Args) > 1 {
		ifaceName = os.Args[1]
	}
	fmt.Printf("Using interface %s\n", ifaceName)

	// Build dataset values
	dataSetValues := []*mms.Value{
		mms.NewInt32(1234),
		mms.NewBinaryTime(false),
		mms.NewInt32(5678),
	}

	// Configure GOOSE communication parameters
	commParams := common.PhyComAddress{
		AppID:        1000,
		DstAddress:   [6]byte{0x01, 0x0C, 0xCD, 0x01, 0x00, 0x01},
		VLANID:       0,
		VLANPriority: 4,
	}

	// Create GOOSE publisher
	pub, err := goose.NewPublisher(goose.PublisherConfig{
		CommParams: commParams,
		Interface:  ifaceName,
	})
	if err != nil {
		fmt.Printf("Failed to create GOOSE publisher: %v\n", err)
		fmt.Println("Reason may be that the Ethernet interface doesn't exist or root permission is required.")
		os.Exit(1)
	}
	defer pub.Close()

	// Configure the publisher
	pub.SetGooseCBRef("simpleIOGenericIO/LLN0$GO$gcbAnalogValues")
	pub.SetConfRev(1)
	pub.SetDataSetRef("simpleIOGenericIO/LLN0$AnalogValues")
	pub.SetTimeAllowedToLive(500)

	// Publish 4 times
	for i := 0; i < 4; i++ {
		time.Sleep(1 * time.Second)

		if i == 3 {
			// On last iteration, add an extra value to send an invalid GOOSE message
			dataSetValues = append(dataSetValues, mms.NewBoolean(true))
		}

		n, err := pub.Publish(dataSetValues)
		if err != nil {
			fmt.Printf("Error sending message: %v\n", err)
		} else {
			fmt.Printf("Sent GOOSE message (%d bytes)\n", n)
		}
	}
}
