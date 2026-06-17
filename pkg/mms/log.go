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

// LogLevel controls the verbosity of MMS / IEC 61850 logging.
type LogLevel int

const (
	// LogNone disables all log output (default).
	LogNone LogLevel = iota
	// LogDebug emits structured IEC 61850 service events (connect, read, write, …).
	// Hex dumps of raw PDU bytes are suppressed.
	LogDebug
	// LogTrace emits everything LogDebug does, plus raw hex dumps of every PDU
	// sent and received (useful for low-level protocol tracing).
	LogTrace
)

// Role identifies which side of the connection produced the log line.
type Role string

const (
	RoleServer Role = "S"
	RoleClient Role = "C"
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
	logMu    sync.RWMutex
	logLevel           = LogNone
	logOut   io.Writer = os.Stderr
)

// SetLogLevel sets the logging verbosity and enables output to os.Stderr.
// Pass LogNone to disable all logging.
//
//	mms.SetLogLevel(mms.LogDebug)  // structured events only
//	mms.SetLogLevel(mms.LogTrace)  // events + raw PDU hex dumps
//	mms.SetLogLevel(mms.LogNone)   // silent (default)
func SetLogLevel(level LogLevel) {
	logMu.Lock()
	logLevel = level
	logMu.Unlock()
}

// SetDebugOutput redirects log output to w (default is os.Stderr).
// Call before SetLogLevel to take effect. Pass nil to revert to os.Stderr.
func SetDebugOutput(w io.Writer) {
	logMu.Lock()
	if w == nil {
		logOut = os.Stderr
	} else {
		logOut = w
	}
	logMu.Unlock()
}

// Logf writes a structured IEC 61850 event line at LogDebug level:
//
//	[IEC61850] [ROLE] EVENT key=value key=value ...
//
// Suppressed when level < LogDebug.
func Logf(role Role, event string, format string, args ...any) {
	logMu.RLock()
	level, w := logLevel, logOut
	logMu.RUnlock()
	if level < LogDebug {
		return
	}
	msg := fmt.Sprintf("[IEC61850] [%s] [%s]", role, event)
	if format != "" {
		msg += " " + fmt.Sprintf(format, args...)
	}
	fmt.Fprintln(w, msg)
}

// DebugHex logs a label and a hex dump of up to 64 bytes at LogTrace level.
// Suppressed when level < LogTrace.
func DebugHex(label string, buf []byte) {
	logMu.RLock()
	level, w := logLevel, logOut
	logMu.RUnlock()
	if level < LogTrace {
		return
	}
	const maxBytes = 64
	display := buf
	suffix := ""
	if len(buf) > maxBytes {
		display = buf[:maxBytes]
		suffix = fmt.Sprintf(" ... (%d bytes total)", len(buf))
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

// debugf writes a low-level MMS protocol line at LogDebug level.
// Used internally for non-service events that do not carry an IEC 61850 event tag.
func debugf(format string, args ...any) {
	logMu.RLock()
	level, w := logLevel, logOut
	logMu.RUnlock()
	if level < LogDebug {
		return
	}
	fmt.Fprintf(w, "[MMS] "+format+"\n", args...)
}
