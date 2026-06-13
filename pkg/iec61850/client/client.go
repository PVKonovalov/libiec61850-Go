/*
 *  client.go
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

// Package client provides an IEC 61850 MMS client implementation.
//
// Usage:
//
//	conn, err := client.Dial("192.168.1.100:102")
//	if err != nil { ... }
//	defer conn.Close()
//
//	// Read a data attribute
//	value, err := conn.ReadObject("simpleIOGenericIO/GGIO1.AnIn1.mag.f", common.FC_MX)
//
//	// Write a data attribute
//	err = conn.WriteObject("simpleIOGenericIO/GGIO1.NamPlt.vendor", common.FC_DC,
//	    mms.NewVisibleString("example"))
//
//	// Enable reporting
//	rcb, err := conn.GetRCBValues("simpleIOGenericIO/LLN0.RP.EventsRCB01")
//	rcb.SetRptEna(true)
//	err = conn.SetRCBValues(rcb, client.RCBElementRptEna, true)
package client

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PVKonovalov/libiec61850-Go/pkg/acse"
	"github.com/PVKonovalov/libiec61850-Go/pkg/asn1ber"
	"github.com/PVKonovalov/libiec61850-Go/pkg/cotp"
	"github.com/PVKonovalov/libiec61850-Go/pkg/iec61850/common"
	"github.com/PVKonovalov/libiec61850-Go/pkg/isopresentation"
	"github.com/PVKonovalov/libiec61850-Go/pkg/isosession"
	"github.com/PVKonovalov/libiec61850-Go/pkg/mms"
)

// Default TCP port for MMS/IEC 61850.
const DefaultPort = 102

// ConnectionState describes the current state of the IED connection.
type ConnectionState int

const (
	StateClosed     ConnectionState = 0
	StateConnecting ConnectionState = 1
	StateConnected  ConnectionState = 2
	StateClosing    ConnectionState = 3
)

// Options holds configuration for a client connection.
type Options struct {
	// Timeout is the request timeout duration. Default: 10s.
	Timeout time.Duration
	// LocalAddress is the optional local IP and port to bind to.
	LocalAddress string
	// LocalPort is the local TCP port. -1 = OS-assigned.
	LocalPort int
	// TLSConfig, if non-nil, enables TLS transport security.
	// (Placeholder – full TLS integration requires tls.Config)
}

// DefaultOptions returns sensible default options.
func DefaultOptions() Options {
	return Options{
		Timeout:   10 * time.Second,
		LocalPort: -1,
	}
}

// Report holds data received in an IEC 61850 report.
type Report struct {
	RCBReference          string
	ReportID              string
	SequenceNumber        uint32
	Timestamp             mms.UTCTime
	DataSetValues         *mms.Value // TypeStructure holding all dataset member values
	ReasonForInclusion    []common.ReasonForInclusion
	HasTimestamp          bool
	HasSeqNum             bool
	HasReasonForInclusion bool
	BufferOverflow        bool
	EntryID               []byte
	ConfRev               uint32
}

// ReportHandler is called for each received report.
type ReportHandler func(report *Report)

// reportSubscription holds a registered report handler for one RCB.
type reportSubscription struct {
	rcbRef  string
	rptID   string
	handler ReportHandler
}

// outstandingCall tracks a pending synchronous request.
type outstandingCall struct {
	invokeID uint32
	response chan []byte
	err      chan error
}

// IedConnection is an IEC 61850 MMS client connection to a server (IED).
type IedConnection struct {
	mu    sync.Mutex
	state ConnectionState
	opts  Options

	cotpConn    *cotp.Conn
	invokeIDSeq atomic.Uint32

	// outstanding synchronous calls: invokeID -> outstandingCall
	pending   map[uint32]*outstandingCall
	pendingMu sync.Mutex

	// registered report handlers
	reportHandlers map[string]*reportSubscription // keyed by rptID

	// channel for receiving PDUs from the background reader
	readErr chan error
}

// Dial creates a new IEC 61850 connection to the given server address (host:port).
// It performs the full COTP + ACSE + MMS handshake.
func Dial(address string) (*IedConnection, error) {
	return DialWithOptions(address, DefaultOptions())
}

// DialWithOptions creates a connection with custom options.
func DialWithOptions(address string, opts Options) (*IedConnection, error) {
	c := &IedConnection{
		opts:           opts,
		state:          StateConnecting,
		pending:        make(map[uint32]*outstandingCall),
		reportHandlers: make(map[string]*reportSubscription),
		readErr:        make(chan error, 1),
	}

	// Establish TCP + COTP connection
	var dialer net.Dialer
	if opts.LocalAddress != "" {
		port := opts.LocalPort
		if port < 0 {
			port = 0
		}
		dialer.LocalAddr = &net.TCPAddr{IP: net.ParseIP(opts.LocalAddress), Port: port}
	}
	if opts.Timeout > 0 {
		dialer.Timeout = opts.Timeout
	}

	cotpOpts := cotp.DefaultOptions()
	tcpConn, err := dialer.Dial("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("iec61850 client: TCP dial %s: %w", address, err)
	}
	c.cotpConn = cotp.NewConn(tcpConn, cotpOpts)
	if err := c.cotpConn.Connect(); err != nil {
		tcpConn.Close()
		return nil, fmt.Errorf("iec61850 client: COTP connect: %w", err)
	}

	// Build connection request PDUs following the full ISO stack:
	// MMS Initiate → ACSE AARQ → ISO Presentation CP → ISO Session CN → COTP

	mmsInitiate := mms.EncodeInitiateRequest()

	aarq, err := acse.BuildAARQ(acse.ConnectionParams{}, mmsInitiate)
	if err != nil {
		c.cotpConn.Close()
		return nil, fmt.Errorf("iec61850 client: build AARQ: %w", err)
	}

	cpPDU := isopresentation.BuildConnectPDU(aarq)
	cnSPDU := isosession.BuildConnectSPDU(cpPDU)

	if err := c.cotpConn.Send(cnSPDU); err != nil {
		c.cotpConn.Close()
		return nil, fmt.Errorf("iec61850 client: send CN SPDU: %w", err)
	}

	// Read and unwrap the Accept response: COTP → Session AC → Presentation CPA → ACSE AARE
	rawAC, err := c.cotpConn.Receive()
	if err != nil {
		c.cotpConn.Close()
		return nil, fmt.Errorf("iec61850 client: receive AC SPDU: %w", err)
	}

	cpaPDU, err := isosession.ParseConnectResponseSPDU(rawAC)
	if err != nil {
		c.cotpConn.Close()
		return nil, fmt.Errorf("iec61850 client: parse session AC: %w", err)
	}

	aareData, err := isopresentation.ParseConnectAcceptPDU(cpaPDU)
	if err != nil {
		c.cotpConn.Close()
		return nil, fmt.Errorf("iec61850 client: parse presentation CPA: %w", err)
	}

	mmsInitResp, err := acse.ParseAARE(aareData)
	if err != nil {
		c.cotpConn.Close()
		return nil, fmt.Errorf("iec61850 client: parse AARE: %w", err)
	}

	if _, _, err := mms.ParseInitiateResponse(mmsInitResp); err != nil {
		c.cotpConn.Close()
		return nil, fmt.Errorf("iec61850 client: MMS initiate: %w", err)
	}

	c.state = StateConnected

	// Start background reader goroutine
	go c.readLoop()

	return c, nil
}

// Close performs an orderly shutdown of the connection.
func (c *IedConnection) Close() error {
	c.mu.Lock()
	if c.state != StateConnected {
		c.mu.Unlock()
		return nil
	}
	c.state = StateClosing
	c.mu.Unlock()

	// Send MMS Conclude Request wrapped in Session+Presentation
	concludeReq := mms.EncodeConcludeRequest()
	wrapped := isosession.WrapDataSPDU(isopresentation.WrapUserData(concludeReq))
	_ = c.cotpConn.Send(wrapped) // best-effort
	return c.cotpConn.Close()
}

// State returns the current connection state.
func (c *IedConnection) State() ConnectionState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

// ReadObject reads the value of a single data attribute from the server.
// objectRef is an IEC 61850 object reference like "LD/LN.DO.DA".
// fc is the functional constraint (e.g., common.FC_MX).
func (c *IedConnection) ReadObject(objectRef string, fc common.FunctionalConstraint) (*mms.Value, error) {
	domainID, itemID, err := objectRefToMMS(objectRef, fc)
	if err != nil {
		return nil, err
	}

	spec := mms.VariableSpecification{DomainID: domainID, ItemID: itemID}
	invokeID := c.nextInvokeID()
	reqPDU := mms.EncodeReadRequest(invokeID, []mms.VariableSpecification{spec})

	respBody, err := c.sendAndReceive(invokeID, reqPDU)
	if err != nil {
		return nil, err
	}

	pduType, err := mms.ParsePDUType(respBody)
	if err != nil {
		return nil, err
	}
	if pduType == 0xA2 { // ConfirmedErrorPDU
		_, mmsErr, err := mms.ParseConfirmedError(respBody)
		if err != nil {
			return nil, err
		}
		return nil, mapMMSError(mmsErr)
	}

	_, svcTag, svcContent, err := mms.ParseConfirmedResponse(respBody)
	if err != nil {
		return nil, err
	}
	if svcTag != 4 { // Read service tag
		return nil, fmt.Errorf("iec61850 client: unexpected service tag %d", svcTag)
	}

	results, err := mms.ParseReadResponse(svcContent)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("iec61850 client: empty read response")
	}
	if results[0].IsError {
		return nil, mapDataAccessError(results[0].Error)
	}
	return results[0].Value, nil
}

// WriteObject writes a value to a data attribute on the server.
func (c *IedConnection) WriteObject(objectRef string, fc common.FunctionalConstraint, value *mms.Value) error {
	domainID, itemID, err := objectRefToMMS(objectRef, fc)
	if err != nil {
		return err
	}

	spec := mms.VariableSpecification{DomainID: domainID, ItemID: itemID}
	invokeID := c.nextInvokeID()
	reqPDU, err := mms.EncodeWriteRequest(invokeID, []mms.VariableSpecification{spec}, []*mms.Value{value})
	if err != nil {
		return err
	}

	respBody, err := c.sendAndReceive(invokeID, reqPDU)
	if err != nil {
		return err
	}

	pduType, err := mms.ParsePDUType(respBody)
	if err != nil {
		return err
	}
	if pduType == 0xA2 { // ConfirmedErrorPDU
		_, mmsErr, _ := mms.ParseConfirmedError(respBody)
		return mapMMSError(mmsErr)
	}
	return nil
}

// GetServerDirectory returns the list of logical device names on the server.
func (c *IedConnection) GetServerDirectory() ([]string, error) {
	invokeID := c.nextInvokeID()
	reqPDU := mms.EncodeGetNameListRequest(invokeID, mms.ObjectClassDomain, "")

	respBody, err := c.sendAndReceive(invokeID, reqPDU)
	if err != nil {
		return nil, err
	}

	pduType, err := mms.ParsePDUType(respBody)
	if err != nil {
		return nil, err
	}
	if pduType == 0xA2 {
		_, mmsErr, _ := mms.ParseConfirmedError(respBody)
		return nil, mapMMSError(mmsErr)
	}

	_, svcTag, svcContent, err := mms.ParseConfirmedResponse(respBody)
	if err != nil {
		return nil, err
	}
	_ = svcTag

	resp, err := mms.ParseGetNameListResponse(svcContent)
	if err != nil {
		return nil, err
	}
	return resp.Names, nil
}

// GetLogicalDeviceDirectory returns the list of logical node names in a logical device.
func (c *IedConnection) GetLogicalDeviceDirectory(ldName string) ([]string, error) {
	invokeID := c.nextInvokeID()
	reqPDU := mms.EncodeGetNameListRequest(invokeID, mms.ObjectClassNamedVariable, ldName)

	respBody, err := c.sendAndReceive(invokeID, reqPDU)
	if err != nil {
		return nil, err
	}

	pduType, err := mms.ParsePDUType(respBody)
	if err != nil {
		return nil, err
	}
	if pduType == 0xA2 {
		_, mmsErr, _ := mms.ParseConfirmedError(respBody)
		return nil, mapMMSError(mmsErr)
	}

	_, _, svcContent, err := mms.ParseConfirmedResponse(respBody)
	if err != nil {
		return nil, err
	}

	resp, err := mms.ParseGetNameListResponse(svcContent)
	if err != nil {
		return nil, err
	}
	return resp.Names, nil
}

// ReadDataSetValues reads all values of a named data set.
func (c *IedConnection) ReadDataSetValues(dataSetRef string, existingDS *DataSet) (*DataSet, error) {
	domainID, itemID, err := dataSetRefToMMS(dataSetRef)
	if err != nil {
		return nil, err
	}

	spec := mms.VariableSpecification{DomainID: domainID, ItemID: itemID}
	invokeID := c.nextInvokeID()
	reqPDU := mms.EncodeReadRequest(invokeID, []mms.VariableSpecification{spec})

	respBody, err := c.sendAndReceive(invokeID, reqPDU)
	if err != nil {
		return nil, err
	}

	pduType, err := mms.ParsePDUType(respBody)
	if err != nil {
		return nil, err
	}
	if pduType == 0xA2 {
		_, mmsErr, _ := mms.ParseConfirmedError(respBody)
		return nil, mapMMSError(mmsErr)
	}

	_, svcTag, svcContent, err := mms.ParseConfirmedResponse(respBody)
	if err != nil {
		return nil, err
	}
	_ = svcTag

	results, err := mms.ParseReadResponse(svcContent)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("iec61850 client: empty data set read response")
	}
	if results[0].IsError {
		return nil, mapDataAccessError(results[0].Error)
	}

	ds := existingDS
	if ds == nil {
		ds = &DataSet{DataSetReference: dataSetRef}
	}
	ds.Values = results[0].Value
	return ds, nil
}

// InstallReportHandler registers a callback for reports from the given RCB.
// rcbRef is the full RCB reference (e.g., "simpleIOGenericIO/LLN0.RP.EventsRCB01").
// rptID is the report ID from the RCB (returned by GetRCBValues).
func (c *IedConnection) InstallReportHandler(rcbRef, rptID string, handler ReportHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reportHandlers[rptID] = &reportSubscription{
		rcbRef:  rcbRef,
		rptID:   rptID,
		handler: handler,
	}
}

// UninstallReportHandler removes the report handler for the given rptID.
func (c *IedConnection) UninstallReportHandler(rptID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.reportHandlers, rptID)
}

// ---- RCB management ----

// RCBElement flags for SetRCBValues.
const (
	RCBElementRptID       uint32 = 0x0001
	RCBElementRptEna      uint32 = 0x0002
	RCBElementDataSet     uint32 = 0x0004
	RCBElementConfRev     uint32 = 0x0008
	RCBElementOptFlds     uint32 = 0x0010
	RCBElementBufTime     uint32 = 0x0020
	RCBElementSqNum       uint32 = 0x0040
	RCBElementTrgOps      uint32 = 0x0080
	RCBElementIntgPd      uint32 = 0x0100
	RCBElementGI          uint32 = 0x0200
	RCBElementPurgeBuf    uint32 = 0x0400
	RCBElementEntryID     uint32 = 0x0800
	RCBElementTimeOfEntry uint32 = 0x1000
	RCBElementResvTms     uint32 = 0x2000
	RCBElementOwner       uint32 = 0x4000
	RCBElementResv        uint32 = 0x8000
)

// ReportControlBlock holds the current parameter values of an RCB.
type ReportControlBlock struct {
	ObjectRef   string
	RptID       string
	RptEna      bool
	Resv        bool
	DataSetRef  string
	ConfRev     uint32
	OptFlds     common.ReportOption
	BufTm       uint32
	SqNum       uint32
	TrgOps      common.TriggerOption
	IntgPd      uint32
	GI          bool
	PurgeBuf    bool
	EntryID     []byte
	TimeOfEntry mms.UTCTime
	ResvTms     int16
	Owner       []byte
	Buffered    bool
}

// DataSet holds the results of a data set read operation.
type DataSet struct {
	DataSetReference string
	Values           *mms.Value // TypeStructure with one element per dataset member
}

// GetDataSetValues returns the individual member values of the data set.
func (ds *DataSet) GetDataSetValues() *mms.Value {
	return ds.Values
}

// GetRCBValues reads the current values of all attributes of an RCB.
func (c *IedConnection) GetRCBValues(rcbRef string) (*ReportControlBlock, error) {
	domainID, itemID, err := objectRefToMMS(rcbRef, common.FC_NONE)
	if err != nil {
		return nil, err
	}
	// RCBs use RP or BR as their domain item prefix
	// The itemID returned from objectRefToMMS without FC won't have $RP$ prefix.
	// We need to detect whether this is a buffered or unbuffered RCB.
	buffered := false
	if len(itemID) > 4 {
		// Detect RP/BR from the path
		for i := 0; i+3 < len(itemID); i++ {
			if itemID[i] == '.' && itemID[i+1] == 'B' && itemID[i+2] == 'R' && itemID[i+3] == '.' {
				buffered = true
				break
			}
		}
	}

	spec := mms.VariableSpecification{DomainID: domainID, ItemID: itemID}
	invokeID := c.nextInvokeID()
	reqPDU := mms.EncodeReadRequest(invokeID, []mms.VariableSpecification{spec})

	respBody, err := c.sendAndReceive(invokeID, reqPDU)
	if err != nil {
		return nil, err
	}

	pduType, err := mms.ParsePDUType(respBody)
	if err != nil {
		return nil, err
	}
	if pduType == 0xA2 {
		_, mmsErr, _ := mms.ParseConfirmedError(respBody)
		return nil, mapMMSError(mmsErr)
	}

	_, _, svcContent, err := mms.ParseConfirmedResponse(respBody)
	if err != nil {
		return nil, err
	}

	results, err := mms.ParseReadResponse(svcContent)
	if err != nil || len(results) == 0 || results[0].IsError {
		if err == nil && len(results) > 0 && results[0].IsError {
			return nil, mapDataAccessError(results[0].Error)
		}
		return nil, fmt.Errorf("iec61850 client: read RCB failed")
	}

	rcb := &ReportControlBlock{ObjectRef: rcbRef, Buffered: buffered}
	parseRCBValue(rcb, results[0].Value)
	return rcb, nil
}

// SetRCBValues writes selected RCB attributes back to the server.
// elements is a bitmask of RCBElement* flags.
// Each attribute is written as an individual variable using a sub-path (e.g., $RptEna).
// RptEna is always written last so the RCB is enabled only after all other fields are set.
// For URCB, Resv=false is automatically prepended when enabling reporting.
func (c *IedConnection) SetRCBValues(rcb *ReportControlBlock, elements uint32, activate bool) error {
	domainID, baseItemID, err := objectRefToMMS(rcb.ObjectRef, common.FC_NONE)
	if err != nil {
		return err
	}

	type attrWrite struct {
		suffix string
		value  *mms.Value
	}

	var attrs []attrWrite

	// Automatically prepend Resv=false for URCB when enabling reporting (not already writing Resv).
	// This releases any prior reservation before taking a new one, matching C library behaviour.
	if !rcb.Buffered && elements&RCBElementRptEna != 0 && rcb.RptEna && elements&RCBElementResv == 0 {
		attrs = append(attrs, attrWrite{"$Resv", mms.NewBoolean(false)})
	}
	if elements&RCBElementResv != 0 {
		attrs = append(attrs, attrWrite{"$Resv", mms.NewBoolean(rcb.Resv)})
	}
	if elements&RCBElementRptID != 0 {
		attrs = append(attrs, attrWrite{"$RptID", mms.NewVisibleString(rcb.RptID)})
	}
	if elements&RCBElementDataSet != 0 {
		attrs = append(attrs, attrWrite{"$DatSet", mms.NewVisibleString(rcb.DataSetRef)})
	}
	if elements&RCBElementOptFlds != 0 {
		attrs = append(attrs, attrWrite{"$OptFlds", optFldsToMMS(rcb.OptFlds)})
	}
	if elements&RCBElementBufTime != 0 {
		attrs = append(attrs, attrWrite{"$BufTm", mms.NewUint32(rcb.BufTm)})
	}
	if elements&RCBElementTrgOps != 0 {
		attrs = append(attrs, attrWrite{"$TrgOps", trgOpsToMMS(rcb.TrgOps)})
	}
	if elements&RCBElementIntgPd != 0 {
		attrs = append(attrs, attrWrite{"$IntgPd", mms.NewUint32(rcb.IntgPd)})
	}
	if elements&RCBElementGI != 0 {
		attrs = append(attrs, attrWrite{"$GI", mms.NewBoolean(rcb.GI)})
	}
	if elements&RCBElementPurgeBuf != 0 {
		attrs = append(attrs, attrWrite{"$PurgeBuf", mms.NewBoolean(rcb.PurgeBuf)})
	}
	// RptEna must be written last — enabling the RCB before other attrs are set causes errors.
	if elements&RCBElementRptEna != 0 {
		attrs = append(attrs, attrWrite{"$RptEna", mms.NewBoolean(rcb.RptEna)})
	}

	if len(attrs) == 0 {
		return nil
	}

	specs := make([]mms.VariableSpecification, len(attrs))
	values := make([]*mms.Value, len(attrs))
	for i, a := range attrs {
		specs[i] = mms.VariableSpecification{DomainID: domainID, ItemID: baseItemID + a.suffix}
		values[i] = a.value
	}

	invokeID := c.nextInvokeID()
	reqPDU, err := mms.EncodeWriteRequest(invokeID, specs, values)
	if err != nil {
		return err
	}

	respBody, err := c.sendAndReceive(invokeID, reqPDU)
	if err != nil {
		return err
	}

	pduType, _ := mms.ParsePDUType(respBody)
	if pduType == 0xA2 {
		_, mmsErr, _ := mms.ParseConfirmedError(respBody)
		return mapMMSError(mmsErr)
	}
	_ = activate
	return nil
}

// ---- internal helpers ----

// nextInvokeID returns a monotonically increasing invocation ID.
func (c *IedConnection) nextInvokeID() uint32 {
	return c.invokeIDSeq.Add(1)
}

// sendAndReceive sends a request PDU and waits for the matching response.
func (c *IedConnection) sendAndReceive(invokeID uint32, reqPDU []byte) ([]byte, error) {
	call := &outstandingCall{
		invokeID: invokeID,
		response: make(chan []byte, 1),
		err:      make(chan error, 1),
	}

	c.pendingMu.Lock()
	c.pending[invokeID] = call
	c.pendingMu.Unlock()

	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, invokeID)
		c.pendingMu.Unlock()
	}()

	// Wrap MMS PDU in Presentation User Data + Session Data SPDU
	wrapped := isosession.WrapDataSPDU(isopresentation.WrapUserData(reqPDU))
	if err := c.cotpConn.Send(wrapped); err != nil {
		return nil, fmt.Errorf("iec61850 client: send request: %w", err)
	}

	timeout := c.opts.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	select {
	case resp := <-call.response:
		return resp, nil
	case err := <-call.err:
		return nil, err
	case <-time.After(timeout):
		return nil, fmt.Errorf("iec61850 client: request timeout (invokeID=%d)", invokeID)
	}
}

// readLoop is the background goroutine that reads incoming PDUs and dispatches them.
func (c *IedConnection) readLoop() {
	for {
		raw, err := c.cotpConn.Receive()
		if err != nil {
			c.mu.Lock()
			c.state = StateClosed
			c.mu.Unlock()
			c.pendingMu.Lock()
			for _, call := range c.pending {
				select {
				case call.err <- err:
				default:
				}
			}
			c.pendingMu.Unlock()
			select {
			case c.readErr <- err:
			default:
			}
			return
		}

		if len(raw) == 0 {
			continue
		}

		// Unwrap ISO Session Data SPDU header ({0x01,0x00,0x01,0x00})
		presPayload, err := isosession.UnwrapDataSPDU(raw)
		if err != nil {
			// Not a data SPDU — could be a session control SPDU; ignore.
			continue
		}

		// Unwrap ISO Presentation User Data (0x61 wrapper)
		data, err := isopresentation.UnwrapUserData(presPayload)
		if err != nil {
			continue
		}

		if len(data) == 0 {
			continue
		}

		pduType := data[0]
		switch pduType {
		case 0xA0, 0xA1, 0xA2: // Confirmed request/response/error
			c.dispatchConfirmedResponse(data)
		case 0xA3: // Unconfirmed (information report)
			c.dispatchInformationReport(data)
		case 0xA8, 0xA9: // Initiate PDUs (shouldn't appear here)
			// ignore
		default:
			// Unknown PDU type – ignore
		}
	}
}

// dispatchConfirmedResponse routes a response PDU to the waiting call.
func (c *IedConnection) dispatchConfirmedResponse(data []byte) {
	var invokeID uint32
	var err error

	if data[0] == 0xA2 { // ConfirmedErrorPDU
		invokeID, _, err = mms.ParseConfirmedError(data)
	} else {
		invokeID, _, _, err = mms.ParseConfirmedResponse(data)
	}
	if err != nil {
		return
	}

	c.pendingMu.Lock()
	call, ok := c.pending[invokeID]
	c.pendingMu.Unlock()
	if !ok {
		return
	}

	select {
	case call.response <- data:
	default:
	}
}

// dispatchInformationReport handles an unsolicited information report (unconfirmed PDU).
func (c *IedConnection) dispatchInformationReport(data []byte) {
	_, content, err := mms.ParseUnconfirmedPDU(data)
	if err != nil {
		return
	}

	report, err := parseInformationReport(content)
	if err != nil {
		return
	}

	c.mu.Lock()
	sub, ok := c.reportHandlers[report.ReportID]
	c.mu.Unlock()

	if ok && sub.handler != nil {
		sub.handler(report)
	}
}

// objectRefToMMS converts an IEC 61850 object reference to MMS domain/item IDs.
func objectRefToMMS(objectRef string, fc common.FunctionalConstraint) (domainID, itemID string, err error) {
	// Format: "LDName/LNName.DOName[.DAName[...]][.FCName]"
	// or with FC: "LDName/LNName$FC$DOName[$DAName]"
	slashIdx := -1
	for i, ch := range objectRef {
		if ch == '/' {
			slashIdx = i
			break
		}
	}
	if slashIdx < 0 {
		return "", "", fmt.Errorf("iec61850 client: missing '/' in %q", objectRef)
	}
	domainID = objectRef[:slashIdx]
	rest := objectRef[slashIdx+1:]

	if fc == common.FC_NONE || fc == common.FC_ALL {
		// Pass through as-is, replacing dots with $ except first
		itemID = rest
		return
	}

	// Replace the first dot (separating LN from DO) with $FC$
	firstDot := -1
	for i, ch := range rest {
		if ch == '.' {
			firstDot = i
			break
		}
	}
	if firstDot < 0 {
		itemID = rest + "$" + fc.String()
		return
	}
	lnPart := rest[:firstDot]
	doPart := rest[firstDot+1:]
	// Replace remaining dots with $ for sub-attributes
	itemID = lnPart + "$" + fc.String() + "$" + replaceDots(doPart)
	return
}

func replaceDots(s string) string {
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			out[i] = '$'
		} else {
			out[i] = s[i]
		}
	}
	return string(out)
}

// dataSetRefToMMS converts a data set reference like "LD/LLN0.DataSetName" to MMS names.
func dataSetRefToMMS(dataSetRef string) (domainID, itemID string, err error) {
	return objectRefToMMS(dataSetRef, common.FC_NONE)
}

// mapMMSError maps an MMS error to an IedClientError.
func mapMMSError(mmsErr mms.Error) error {
	switch mmsErr {
	case mms.ErrAccessObjectNonExistent:
		return common.ErrorObjectDoesNotExist
	case mms.ErrAccessObjectAccessDenied:
		return common.ErrorAccessDenied
	case mms.ErrServiceTimeout:
		return common.ErrorTimeout
	default:
		return fmt.Errorf("iec61850 client: MMS error %d", int(mmsErr))
	}
}

// mapDataAccessError maps an MMS DataAccessError to a client error.
func mapDataAccessError(dae mms.DataAccessError) error {
	switch dae {
	case mms.DataAccessErrorObjectNonExistent:
		return common.ErrorObjectDoesNotExist
	case mms.DataAccessErrorObjectAccessDenied:
		return common.ErrorAccessDenied
	case mms.DataAccessErrorObjectValueInvalid:
		return common.ErrorObjectValueInvalid
	default:
		return fmt.Errorf("iec61850 client: data access error %d", int(dae))
	}
}

// parseRCBValue parses a structure value returned by reading an RCB into an RCB struct.
// The RCB structure members are in a fixed order defined by IEC 61850-7-2.
// URCB order: RptID, RptEna, Resv, DatSet, ConfRev, OptFlds, BufTm, SqNum, TrgOps, IntgPd, GI
func parseRCBValue(rcb *ReportControlBlock, v *mms.Value) {
	if v == nil || v.Type() != mms.TypeStructure {
		return
	}
	for i := 0; i < v.Size(); i++ {
		elem := v.GetElement(i)
		if elem == nil {
			continue
		}
		switch i {
		case 0: // RptID
			if elem.Type() == mms.TypeVisibleString {
				rcb.RptID = elem.GetVisibleString()
			}
		case 1: // RptEna
			if elem.Type() == mms.TypeBoolean {
				rcb.RptEna = elem.GetBoolean()
			}
		case 2: // Resv
			if elem.Type() == mms.TypeBoolean {
				rcb.Resv = elem.GetBoolean()
			}
		case 3: // DatSet
			if elem.Type() == mms.TypeVisibleString {
				rcb.DataSetRef = elem.GetVisibleString()
			}
		case 4: // ConfRev
			if elem.Type() == mms.TypeUnsigned || elem.Type() == mms.TypeInteger {
				rcb.ConfRev = elem.GetUint32()
			}
		case 5: // OptFlds
			if elem.Type() == mms.TypeBitString {
				bits, _ := elem.GetBitString()
				rcb.OptFlds = optFldsFromMMS(bits)
			}
		case 6: // BufTm
			if elem.Type() == mms.TypeUnsigned || elem.Type() == mms.TypeInteger {
				rcb.BufTm = elem.GetUint32()
			}
		case 7: // SqNum
			if elem.Type() == mms.TypeUnsigned || elem.Type() == mms.TypeInteger {
				rcb.SqNum = elem.GetUint32()
			}
		case 8: // TrgOps
			if elem.Type() == mms.TypeBitString {
				bits, _ := elem.GetBitString()
				rcb.TrgOps = trgOpsFromMMS(bits)
			}
		case 9: // IntgPd
			if elem.Type() == mms.TypeUnsigned || elem.Type() == mms.TypeInteger {
				rcb.IntgPd = elem.GetUint32()
			}
		case 10: // GI (URCB) or PurgeBuf (BRCB)
			if elem.Type() == mms.TypeBoolean {
				if rcb.Buffered {
					rcb.PurgeBuf = elem.GetBoolean()
				} else {
					rcb.GI = elem.GetBoolean()
				}
			}
		}
	}
}

// trgOpsToMMS converts a TriggerOption bitmask to an MMS 6-bit BitString.
// IEC 61850 TrgOps bit positions (MSB-first): 0=reserved, 1=dchg, 2=qchg, 3=dupd, 4=integrity, 5=gi.
func trgOpsToMMS(trgOps common.TriggerOption) *mms.Value {
	var b byte
	if trgOps&common.TriggerDataChanged != 0 {
		b |= 0x40 // bit-string position 1 → byte bit 6
	}
	if trgOps&common.TriggerQualityChanged != 0 {
		b |= 0x20 // position 2 → byte bit 5
	}
	if trgOps&common.TriggerDataUpdate != 0 {
		b |= 0x10 // position 3 → byte bit 4
	}
	if trgOps&common.TriggerIntegrity != 0 {
		b |= 0x08 // position 4 → byte bit 3
	}
	if trgOps&common.TriggerGI != 0 {
		b |= 0x04 // position 5 → byte bit 2
	}
	return mms.NewBitString([]byte{b}, 6)
}

// trgOpsFromMMS decodes an MMS 6-bit BitString back into a TriggerOption bitmask.
func trgOpsFromMMS(bits []byte) common.TriggerOption {
	if len(bits) == 0 {
		return 0
	}
	b := bits[0]
	var v common.TriggerOption
	if b&0x40 != 0 {
		v |= common.TriggerDataChanged
	}
	if b&0x20 != 0 {
		v |= common.TriggerQualityChanged
	}
	if b&0x10 != 0 {
		v |= common.TriggerDataUpdate
	}
	if b&0x08 != 0 {
		v |= common.TriggerIntegrity
	}
	if b&0x04 != 0 {
		v |= common.TriggerGI
	}
	return v
}

// optFldsToMMS converts a ReportOption bitmask to an MMS 10-bit BitString.
// IEC 61850 OptFlds bit positions (MSB-first): 0=reserved, 1=seqNum, 2=timeStamp,
// 3=reasonCode, 4=dataSetName, 5=dataRef, 6=bufOverflow, 7=entryId, 8=confRev, 9=segFlag.
func optFldsToMMS(optFlds common.ReportOption) *mms.Value {
	var byte0, byte1 byte
	if optFlds&common.ReportOptSeqNum != 0 {
		byte0 |= 0x40
	}
	if optFlds&common.ReportOptTimeStamp != 0 {
		byte0 |= 0x20
	}
	if optFlds&common.ReportOptReasonForInclusion != 0 {
		byte0 |= 0x10
	}
	if optFlds&common.ReportOptDataSet != 0 {
		byte0 |= 0x08
	}
	if optFlds&common.ReportOptDataReference != 0 {
		byte0 |= 0x04
	}
	if optFlds&common.ReportOptBufferOverflow != 0 {
		byte0 |= 0x02
	}
	if optFlds&common.ReportOptEntryID != 0 {
		byte0 |= 0x01
	}
	if optFlds&common.ReportOptConfRev != 0 {
		byte1 |= 0x80
	}
	return mms.NewBitString([]byte{byte0, byte1}, 10)
}

// optFldsFromMMS decodes an MMS BitString back into a ReportOption bitmask.
func optFldsFromMMS(bits []byte) common.ReportOption {
	var v common.ReportOption
	if len(bits) >= 1 {
		b := bits[0]
		if b&0x40 != 0 {
			v |= common.ReportOptSeqNum
		}
		if b&0x20 != 0 {
			v |= common.ReportOptTimeStamp
		}
		if b&0x10 != 0 {
			v |= common.ReportOptReasonForInclusion
		}
		if b&0x08 != 0 {
			v |= common.ReportOptDataSet
		}
		if b&0x04 != 0 {
			v |= common.ReportOptDataReference
		}
		if b&0x02 != 0 {
			v |= common.ReportOptBufferOverflow
		}
		if b&0x01 != 0 {
			v |= common.ReportOptEntryID
		}
	}
	if len(bits) >= 2 && bits[1]&0x80 != 0 {
		v |= common.ReportOptConfRev
	}
	return v
}

// decodeReasonBits decodes an MMS BitString into a ReasonForInclusion.
// IEC 61850 bit positions (MSB-first): 0=reserved, 1=dchg, 2=qchg, 3=dupd, 4=integrity, 5=gi, 6=appTrigger.
func decodeReasonBits(bits []byte) common.ReasonForInclusion {
	if len(bits) == 0 {
		return common.ReasonNotIncluded
	}
	b := bits[0]
	switch {
	case b&0x40 != 0:
		return common.ReasonDataChange
	case b&0x20 != 0:
		return common.ReasonQualityChange
	case b&0x10 != 0:
		return common.ReasonDataUpdate
	case b&0x08 != 0:
		return common.ReasonIntegrity
	case b&0x04 != 0:
		return common.ReasonGI
	case b&0x02 != 0:
		return common.ReasonApplicationTrigger
	default:
		return common.ReasonNotIncluded
	}
}

// parseInformationReport parses an unconfirmed PDU's unconfirmedService content.
// Wire structure (from ok.pcap):
//
//	a3 { a0 { a1 05 { 80 03 "RPT" }   ← InformationReport service header (variableListName)
//	           a0 nn { accessResults } ← listOfAccessResult } }
//
// dispatchInformationReport passes the a0 (unconfirmedService) inner value, which contains
// the a1 header sibling and the a0 listOfAccessResult sibling at the same nesting level.
func parseInformationReport(content []byte) (*Report, error) {
	var listData []byte
	offset := 0
	for offset < len(content) {
		tlv, newOff, err := asn1ber.ParseTLV(content, offset)
		if err != nil {
			break
		}
		offset = newOff
		if tlv.Class == asn1ber.ClassContext && tlv.Constructed {
			if tlv.Tag == 0 { // [0] CONSTRUCTED = listOfAccessResult
				listData = tlv.Value
			}
			// tlv.Tag == 1: informationReport service header (variableListName "RPT") – skip
		}
	}
	if listData == nil {
		return nil, fmt.Errorf("iec61850 client: no listOfAccessResult in unconfirmed PDU")
	}
	return parseReportAccessResults(listData)
}

// parseReportAccessResults parses the listOfAccessResult from an MMS InformationReport.
// IEC 61850-7-2 report field order: rptID, optFlds, [seqNum], [timeOfEntry],
// [dataSetName], [bufOvfl], [confRev], inclusionBitstr, per-member {[dataRef] value [reason]}.
func parseReportAccessResults(data []byte) (*Report, error) {
	report := &Report{}
	offset := 0

	// 1. RptID (VisibleString, always present)
	if offset < len(data) {
		v, newOff, err := mms.DecodeValue(data, offset)
		if err != nil {
			return nil, fmt.Errorf("iec61850 client: report rptID: %w", err)
		}
		offset = newOff
		if v.Type() == mms.TypeVisibleString {
			report.ReportID = v.GetVisibleString()
		}
	}

	// 2. OptFlds (BitString, always present)
	var optFlds common.ReportOption
	if offset < len(data) {
		v, newOff, err := mms.DecodeValue(data, offset)
		if err == nil && v.Type() == mms.TypeBitString {
			offset = newOff
			bits, _ := v.GetBitString()
			optFlds = optFldsFromMMS(bits)
		}
	}

	// 3. SeqNum (UNSIGNED, if optFlds.seqNum)
	if optFlds&common.ReportOptSeqNum != 0 && offset < len(data) {
		v, newOff, err := mms.DecodeValue(data, offset)
		if err == nil {
			offset = newOff
			if v.Type() == mms.TypeUnsigned || v.Type() == mms.TypeInteger {
				report.SequenceNumber = v.GetUint32()
				report.HasSeqNum = true
			}
		}
	}

	// 4. TimeOfEntry (BinaryTime, if optFlds.timeStamp)
	if optFlds&common.ReportOptTimeStamp != 0 && offset < len(data) {
		_, newOff, err := mms.DecodeValue(data, offset)
		if err == nil {
			offset = newOff
			report.HasTimestamp = true
		}
	}

	// 5. DataSetName (VisibleString, if optFlds.dataSetName)
	if optFlds&common.ReportOptDataSet != 0 && offset < len(data) {
		v, newOff, err := mms.DecodeValue(data, offset)
		if err == nil && v.Type() == mms.TypeVisibleString {
			offset = newOff
			report.RCBReference = v.GetVisibleString()
		}
	}

	// 6. BufOvfl (BOOLEAN, if optFlds.bufOverflow)
	if optFlds&common.ReportOptBufferOverflow != 0 && offset < len(data) {
		v, newOff, err := mms.DecodeValue(data, offset)
		if err == nil {
			offset = newOff
			if v.Type() == mms.TypeBoolean {
				report.BufferOverflow = v.GetBoolean()
			}
		}
	}

	// 7. ConfRev (UNSIGNED, if optFlds.confRev)
	if optFlds&common.ReportOptConfRev != 0 && offset < len(data) {
		v, newOff, err := mms.DecodeValue(data, offset)
		if err == nil {
			offset = newOff
			if v.Type() == mms.TypeUnsigned || v.Type() == mms.TypeInteger {
				report.ConfRev = v.GetUint32()
			}
		}
	}

	// 8. InclusionBitstring (BitString)
	var inclusionBits []byte
	var inclusionNumBits int
	if offset < len(data) {
		v, newOff, err := mms.DecodeValue(data, offset)
		if err == nil && v.Type() == mms.TypeBitString {
			offset = newOff
			inclusionBits, inclusionNumBits = v.GetBitString()
		}
	}

	// 9. Per-member data — iterate over all dataset slots (inclusionNumBits total)
	var values []*mms.Value
	var reasons []common.ReasonForInclusion

	for i := 0; i < inclusionNumBits; i++ {
		byteIdx := i / 8
		bitPos := 7 - (i % 8)
		included := byteIdx < len(inclusionBits) && (inclusionBits[byteIdx]>>uint(bitPos))&1 == 1

		if !included {
			values = append(values, nil)
			reasons = append(reasons, common.ReasonNotIncluded)
			continue
		}

		// DataReference (VisibleString, if optFlds.dataRef) — consume but ignore
		if optFlds&common.ReportOptDataReference != 0 && offset < len(data) {
			v, newOff, err := mms.DecodeValue(data, offset)
			if err == nil && v.Type() == mms.TypeVisibleString {
				offset = newOff
			}
		}

		// Value (MMS Data)
		var val *mms.Value
		if offset < len(data) {
			v, newOff, err := mms.DecodeValue(data, offset)
			if err != nil {
				break
			}
			offset = newOff
			val = v
		}
		values = append(values, val)

		// ReasonForInclusion (BitString, if optFlds.reason)
		reason := common.ReasonNotIncluded
		if optFlds&common.ReportOptReasonForInclusion != 0 && offset < len(data) {
			v, newOff, err := mms.DecodeValue(data, offset)
			if err == nil && v.Type() == mms.TypeBitString {
				offset = newOff
				bits, _ := v.GetBitString()
				reason = decodeReasonBits(bits)
			}
		}
		reasons = append(reasons, reason)
	}

	report.DataSetValues = mms.NewStructure(values)
	report.ReasonForInclusion = reasons
	return report, nil
}
