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

// Role identifies which side of the connection produced the log line.
type Role string

const (
	RoleServer Role = "SERVER"
	RoleClient Role = "CLIENT"
)

// IEC 61850 / MMS event name constants used as the structured event tag.
// Format mirrors the C library's DEBUG_MMS_SERVER printfs, converted to
// uppercase underscore identifiers for easy grepping and log filtering.
const (
	EventConnect        = "IEC61850_CONNECT"           // client connected / association accepted
	EventDisconnect     = "IEC61850_DISCONNECT"        // client disconnected / association released
	EventInitiate       = "IEC61850_INITIATE"          // MMS Initiate request/response
	EventRead           = "IEC61850_READ"              // Read service (specific variable)
	EventReadNameList   = "IEC61850_READ_NAMELIST_VAR" // GetNameList for named variables
	EventReadNameListDS = "IEC61850_READ_NAMELIST_DS"  // GetNameList for named variable lists (datasets)
	EventReadNameListLD = "IEC61850_READ_NAMELIST_LD"  // GetNameList for logical devices (domains)
	EventReadVarAttr    = "IEC61850_READ_VAR_ATTR"     // GetVariableAccessAttributes (type query)
	EventReadDsAttr     = "IEC61850_READ_DS_ATTR"      // GetNamedVariableListAttributes (dataset members)
	EventWrite          = "IEC61850_WRITE"             // Write service
	EventIdentify       = "IEC61850_IDENTIFY"          // Identify service
	EventError          = "IEC61850_ERROR"             // service error response sent
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

// SetDebug is a convenience wrapper that enables debug logging to os.Stderr
// when enable is true, and disables it when false.
func SetDebug(enable bool) {
	if enable {
		SetDebugOutput(os.Stderr)
	} else {
		SetDebugOutput(nil)
	}
}

// Logf writes a structured debug line in the format:
//
//	[IEC61850] [ROLE] event key=value key=value ...
//
// role is RoleServer or RoleClient.
// event is one of the Event* constants (e.g. EventRead).
// format/args are optional key=value pairs, formatted with fmt.Sprintf.
//
// Output is suppressed when no debug output is configured.
func Logf(role Role, event string, format string, args ...any) {
	debugMu.RLock()
	w := debugOutput
	debugMu.RUnlock()
	if w == nil {
		return
	}
	msg := fmt.Sprintf("[IEC61850] [%s] %s", role, event)
	if format != "" {
		msg += " " + fmt.Sprintf(format, args...)
	}
	fmt.Fprintln(w, msg)
}

// debugf writes a low-level MMS protocol debug line (not IEC 61850 service level).
// Use Logf for IEC 61850 service events; reserve debugf for raw PDU tracing.
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
