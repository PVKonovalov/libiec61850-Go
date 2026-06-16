/*
 *  log.go
 *
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
	"fmt"
	"io"
	"os"
	"sync"
)

var (
	debugMu     sync.RWMutex
	debugOutput io.Writer
)

// SetDebugOutput enables or disables MMS debug logging.
// Pass os.Stderr or any io.Writer to enable; pass nil to disable.
// Safe to call from multiple goroutines.
func SetDebugOutput(w io.Writer) {
	debugMu.Lock()
	debugOutput = w
	debugMu.Unlock()
}

// SetDebug is a convenience function that enables debug logging to os.Stderr
// when enable is true, and disables it when false.
func SetDebug(enable bool) {
	if enable {
		SetDebugOutput(os.Stderr)
	} else {
		SetDebugOutput(nil)
	}
}

// debugf writes a formatted debug line if debug output is configured.
func debugf(format string, args ...any) {
	debugMu.RLock()
	w := debugOutput
	debugMu.RUnlock()
	if w == nil {
		return
	}
	fmt.Fprintf(w, "[MMS] "+format+"\n", args...)
}

// DebugHex logs a label and a hex dump of up to 64 bytes.
func DebugHex(label string, buf []byte) {
	debugMu.RLock()
	w := debugOutput
	debugMu.RUnlock()
	if w == nil {
		return
	}
	maxBytes := 64
	suffix := ""
	display := buf
	if len(buf) > maxBytes {
		suffix = fmt.Sprintf(" ... (%d bytes total)", len(buf))
		display = buf[:maxBytes]
	}
	hex := ""
	for i, b := range display {
		if i > 0 {
			hex += " "
		}
		hex += fmt.Sprintf("%02x", b)
	}
	fmt.Fprintf(w, "[MMS] %s: %s%s\n", label, hex, suffix)
}
