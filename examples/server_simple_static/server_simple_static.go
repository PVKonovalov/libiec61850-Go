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
// The data model is loaded from the auto-generated internal/model package which
// was produced from sampleModel_with_dataset.cid via ModelGenerator gengolibmodel.
//
// Usage:
//
//	./server_simple_static [--debug] [port]
//
// Default port: 102
package main

import (
	"fmt"
	"math"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	generatedModel "github.com/PVKonovalov/libiec61850-Go/internal/model"
	"github.com/PVKonovalov/libiec61850-Go/pkg/iec61850/common"
	imodel "github.com/PVKonovalov/libiec61850-Go/pkg/iec61850/model"
	"github.com/PVKonovalov/libiec61850-Go/pkg/iec61850/server"
	"github.com/PVKonovalov/libiec61850-Go/pkg/mms"
)

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
		mms.SetLogLevel(mms.LogTrace)
	}

	iedModel := generatedModel.BuildModel()

	totWMagF, _ := iedModel.FindNode("Device1/MMXU2.TotW.mag.f").(*imodel.DataAttribute)
	totWT, _ := iedModel.FindNode("Device1/MMXU2.TotW.t").(*imodel.DataAttribute)
	valQ, _ := iedModel.FindNode("Device1/MMXU2.TotW.q").(*imodel.DataAttribute)

	iedServer := server.NewIedServer(iedModel, nil)
	if err := iedServer.Start("0.0.0.0", port); err != nil {
		fmt.Printf("Starting server failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Server started on port %d\n", port)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	start := time.Now()
	updateTicker := time.NewTicker(1 * time.Second)
	count := 0
	for {
		select {
		case <-sigCh:
			updateTicker.Stop()
			goto exit
		case <-updateTicker.C:
			// Update measurement values every second.
			// Set t and q directly (no trigger) so they are bundled into the
			// single report fired by the mag.f UpdateAttributeValue call.
			now := time.Now()
			elapsed := now.Sub(start).Seconds()
			v := float32(math.Sin(2 * math.Pi * elapsed / 30.0))
			if totWT != nil {
				totWT.Value = mms.NewUTCTime(mms.UTCTimeFromTime(now))
			}
			if valQ != nil {
				if count%10 == 0 {
					// every 10 seconds, set quality to bad
					valQ.Value = mms.NewQuality(common.QualityInvalid)
				} else {
					// otherwise, set quality to good
					valQ.Value = mms.NewQuality(common.QualityGood)
				}
			}
			if totWMagF != nil {
				iedServer.UpdateAttributeValue(totWMagF, mms.NewFloat32(v))
			}
			count++
		}
	}

exit:
	fmt.Println("Stopping server...")
	iedServer.Stop()
}
