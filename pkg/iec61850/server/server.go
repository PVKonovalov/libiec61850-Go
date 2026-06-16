/*
 *  server.go
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

// Package server provides an IEC 61850 MMS server implementation.
//
// The server hosts an IED data model and responds to client requests
// for reading/writing data attributes, managing data sets, and
// subscribing to reports.
//
// Usage:
//
//	model := model.NewIedModel("testIED")
//	ld := model.NewLogicalDevice("simpleIO", model)
//	ln := model.NewLogicalNode("GGIO1", ld)
//	// ... add data objects and attributes ...
//
//	server := server.NewIedServer(model, nil)
//	server.Start(102)
//	// ... update values ...
//	server.Stop()
package server

import (
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/PVKonovalov/libiec61850-Go/pkg/cotp"
	"github.com/PVKonovalov/libiec61850-Go/pkg/iec61850/common"
	"github.com/PVKonovalov/libiec61850-Go/pkg/iec61850/model"
	"github.com/PVKonovalov/libiec61850-Go/pkg/isopresentation"
	"github.com/PVKonovalov/libiec61850-Go/pkg/isosession"
	"github.com/PVKonovalov/libiec61850-Go/pkg/mms"
)

// Config holds configuration parameters for the IED server.
type Config struct {
	// ReportBufferSize is the buffer size for each buffered RCB in bytes.
	ReportBufferSize int
	// MaxConnections is the maximum number of simultaneous MMS connections.
	// 0 = use default (10).
	MaxConnections int
	// Edition is the IEC 61850 edition (0=Ed1, 1=Ed2, 2=Ed2.1).
	Edition uint8
	// FileServiceBasePath is the directory served by MMS file services.
	FileServiceBasePath string
	// EnableFileService enables the MMS file service.
	EnableFileService bool
	// EnableDynamicDataSets allows clients to create temporary data sets.
	EnableDynamicDataSets bool
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		ReportBufferSize:      65536,
		MaxConnections:        10,
		Edition:               common.Edition2,
		EnableFileService:     true,
		EnableDynamicDataSets: true,
	}
}

// WriteAccessHandler is called before a client writes a data attribute.
// Return nil to allow the write, or an error to reject it.
type WriteAccessHandler func(da *model.DataAttribute, value *mms.Value, clientAddr net.Addr) error

// ReadAccessHandler is called before a client reads a data attribute.
// Return nil to allow the read, or an error to reject it.
type ReadAccessHandler func(da *model.DataAttribute, clientAddr net.Addr) error

// brcbState holds the runtime MMS values for one buffered RCB instance.
// Field order matches createBufferedReportControlBlock() in reporting.c.
type brcbState struct {
	rptID       *mms.Value // [0] MMS_VISIBLE_STRING -129
	rptEna      *mms.Value // [1] MMS_BOOLEAN
	datSet      *mms.Value // [2] MMS_VISIBLE_STRING -129
	confRev     *mms.Value // [3] MMS_UNSIGNED 32
	optFlds     *mms.Value // [4] MMS_BIT_STRING -10
	bufTm       *mms.Value // [5] MMS_UNSIGNED 32
	sqNum       *mms.Value // [6] MMS_UNSIGNED 16
	trgOps      *mms.Value // [7] MMS_BIT_STRING -6
	intgPd      *mms.Value // [8] MMS_UNSIGNED 32
	gi          *mms.Value // [9] MMS_BOOLEAN
	purgeBuf    *mms.Value // [10] MMS_BOOLEAN
	entryID     *mms.Value // [11] MMS_OCTET_STRING 8
	timeofEntry *mms.Value // [12] MMS_BINARY_TIME 6
}

func (s *brcbState) fieldNames() []string {
	return []string{"RptID", "RptEna", "DatSet", "ConfRev", "OptFlds", "BufTm", "SqNum", "TrgOps", "IntgPd", "GI", "PurgeBuf", "EntryID", "TimeofEntry"}
}

func (s *brcbState) asValues() []*mms.Value {
	return []*mms.Value{s.rptID, s.rptEna, s.datSet, s.confRev, s.optFlds, s.bufTm, s.sqNum, s.trgOps, s.intgPd, s.gi, s.purgeBuf, s.entryID, s.timeofEntry}
}

func (s *brcbState) fieldValue(name string) *mms.Value {
	switch name {
	case "RptID":
		return s.rptID
	case "RptEna":
		return s.rptEna
	case "DatSet":
		return s.datSet
	case "ConfRev":
		return s.confRev
	case "OptFlds":
		return s.optFlds
	case "BufTm":
		return s.bufTm
	case "SqNum":
		return s.sqNum
	case "TrgOps":
		return s.trgOps
	case "IntgPd":
		return s.intgPd
	case "GI":
		return s.gi
	case "PurgeBuf":
		return s.purgeBuf
	case "EntryID":
		return s.entryID
	case "TimeofEntry":
		return s.timeofEntry
	}
	return nil
}

func (s *brcbState) setFieldValue(name string, v *mms.Value) bool {
	switch name {
	case "RptID":
		s.rptID = v
	case "RptEna":
		s.rptEna = v
	case "DatSet":
		s.datSet = v
	case "ConfRev":
		s.confRev = v
	case "OptFlds":
		s.optFlds = v
	case "BufTm":
		s.bufTm = v
	case "SqNum":
		s.sqNum = v
	case "TrgOps":
		s.trgOps = v
	case "IntgPd":
		s.intgPd = v
	case "GI":
		s.gi = v
	case "PurgeBuf":
		s.purgeBuf = v
	case "EntryID":
		s.entryID = v
	case "TimeofEntry":
		s.timeofEntry = v
	default:
		return false
	}
	return true
}

// encodeTrgOpsBits converts a TriggerOption bitmask to MMS BIT_STRING(-6) bytes.
// Mirrors MmsValue_setBitStringBit calls in createTrgOps() in reporting.c.
func encodeTrgOpsBits(t common.TriggerOption) []byte {
	var b byte
	if t&common.TriggerDataChanged != 0 {
		b |= 0x40 // bit 1
	}
	if t&common.TriggerQualityChanged != 0 {
		b |= 0x20 // bit 2
	}
	if t&common.TriggerDataUpdate != 0 {
		b |= 0x10 // bit 3
	}
	if t&common.TriggerIntegrity != 0 {
		b |= 0x08 // bit 4
	}
	if t&common.TriggerGI != 0 {
		b |= 0x04 // bit 5
	}
	return []byte{b}
}

// encodeOptFldsBits converts a ReportOption bitmask to MMS BIT_STRING(-10) bytes.
// Mirrors MmsValue_setBitStringBit calls in createOptFlds() in reporting.c.
func encodeOptFldsBits(o common.ReportOption) []byte {
	var b0, b1 byte
	if o&common.ReportOptSeqNum != 0 {
		b0 |= 0x40 // bit 1
	}
	if o&common.ReportOptTimeStamp != 0 {
		b0 |= 0x20 // bit 2
	}
	if o&common.ReportOptReasonForInclusion != 0 {
		b0 |= 0x10 // bit 3
	}
	if o&common.ReportOptDataSet != 0 {
		b0 |= 0x08 // bit 4
	}
	if o&common.ReportOptDataReference != 0 {
		b0 |= 0x04 // bit 5
	}
	if o&common.ReportOptBufferOverflow != 0 {
		b0 |= 0x02 // bit 6
	}
	if o&common.ReportOptEntryID != 0 {
		b0 |= 0x01 // bit 7
	}
	if o&common.ReportOptConfRev != 0 {
		b1 |= 0x80 // bit 8
	}
	return []byte{b0, b1}
}

// newBRCBState initialises runtime BRCB state from static model configuration.
// datSetMmsRef is the full MMS DatSet reference (e.g. "SampleIEDDevice1/LLN0$dataset1").
func newBRCBState(rcb *model.ReportControlBlock, datSetMmsRef string) *brcbState {
	rptID := rcb.RptID
	if rptID == "" {
		rptID = rcb.LDInst + "/" + rcb.LNName + "$BR$" + rcb.Name
	}
	return &brcbState{
		rptID:       mms.NewVisibleString(rptID),
		rptEna:      mms.NewBoolean(false),
		datSet:      mms.NewVisibleString(datSetMmsRef),
		confRev:     mms.NewUint32(rcb.ConfRev),
		optFlds:     mms.NewBitString(encodeOptFldsBits(rcb.OptFields), -10),
		bufTm:       mms.NewUint32(rcb.BufTime),
		sqNum:       mms.NewUint32(0),
		trgOps:      mms.NewBitString(encodeTrgOpsBits(rcb.TrgOps), -6),
		intgPd:      mms.NewUint32(rcb.IntgPd),
		gi:          mms.NewBoolean(false),
		purgeBuf:    mms.NewBoolean(false),
		entryID:     mms.NewOctetString(make([]byte, 8)),
		timeofEntry: mms.NewBinaryTime(true),
	}
}

// IedServer is an IEC 61850 MMS server.
type IedServer struct {
	mu      sync.Mutex
	model   *model.IedModel
	config  *Config
	running bool

	listener *cotp.Listener
	conns    map[uint64]*serverConn
	connID   atomic.Uint64

	writeHandler WriteAccessHandler
	readHandler  ReadAccessHandler

	brcbValues map[string]*brcbState // keyed by RCB.Name (e.g. "LLN0_Events_BuffRep01")
}

// NewIedServer creates a new IED server with the given data model and configuration.
// If config is nil, DefaultConfig() is used.
func NewIedServer(iedModel *model.IedModel, config *Config) *IedServer {
	if config == nil {
		config = DefaultConfig()
	}
	s := &IedServer{
		model:      iedModel,
		config:     config,
		conns:      make(map[uint64]*serverConn),
		brcbValues: make(map[string]*brcbState),
	}
	// Initialise runtime state for each buffered RCB.
	// DatSet MMS reference = IEDName+LDInst+"/"+LNName+"$"+DataSetRef
	for _, rcb := range iedModel.RCBs {
		if !rcb.Buffered {
			continue
		}
		mmsDomain := iedModel.Name() + rcb.LDInst
		datSetRef := ""
		if rcb.DataSetRef != "" {
			datSetRef = mmsDomain + "/" + rcb.LNName + "$" + rcb.DataSetRef
		}
		s.brcbValues[rcb.Name] = newBRCBState(rcb, datSetRef)
	}
	return s
}

// SetWriteAccessHandler installs a handler for write access control.
func (s *IedServer) SetWriteAccessHandler(h WriteAccessHandler) {
	s.mu.Lock()
	s.writeHandler = h
	s.mu.Unlock()
}

// SetReadAccessHandler installs a handler for read access control.
func (s *IedServer) SetReadAccessHandler(h ReadAccessHandler) {
	s.mu.Lock()
	s.readHandler = h
	s.mu.Unlock()
}

// Start begins accepting connections on the given TCP port.
// It spawns a background goroutine for the accept loop.
func (s *IedServer) Start(address string, port int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("iec61850 server: already running")
	}

	addr := fmt.Sprintf("%s:%d", address, port)
	l, err := cotp.ListenTCP(addr, cotp.DefaultOptions())
	if err != nil {
		return fmt.Errorf("iec61850 server: listen on %s: %w", addr, err)
	}

	s.listener = l
	s.running = true

	go s.acceptLoop()
	return nil
}

// Stop closes the server listener and all active client connections.
func (s *IedServer) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}
	s.running = false
	s.listener.Close()

	for _, conn := range s.conns {
		conn.close()
	}
}

// IsRunning returns true if the server is accepting connections.
func (s *IedServer) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// UpdateAttributeValue updates a data attribute value in the model.
// This is how the server-side application sets new process values.
func (s *IedServer) UpdateAttributeValue(da *model.DataAttribute, value *mms.Value) {
	da.Value = value
	// TODO: trigger reporting for subscribed clients when trigger options match
}

// LockDataModel acquires a read lock on the data model.
// Callers should call UnlockDataModel when done.
func (s *IedServer) LockDataModel() {
	s.mu.Lock()
}

// UnlockDataModel releases the data model lock.
func (s *IedServer) UnlockDataModel() {
	s.mu.Unlock()
}

// ---- accept loop ----

func (s *IedServer) acceptLoop() {
	for {
		cotpConn, err := s.listener.Accept()
		if err != nil {
			s.mu.Lock()
			running := s.running
			s.mu.Unlock()
			if !running {
				return
			}
			continue
		}

		id := s.connID.Add(1)
		conn := &serverConn{
			id:     id,
			server: s,
			cotp:   cotpConn,
		}

		s.mu.Lock()
		if len(s.conns) >= s.config.MaxConnections {
			s.mu.Unlock()
			cotpConn.Close()
			continue
		}
		s.conns[id] = conn
		s.mu.Unlock()

		go conn.handle()
	}
}

func (s *IedServer) removeConn(id uint64) {
	s.mu.Lock()
	delete(s.conns, id)
	s.mu.Unlock()
}

// ---- per-connection handler ----

// serverConn handles one MMS client connection.
type serverConn struct {
	id     uint64
	server *IedServer
	cotp   *cotp.Conn
}

func (c *serverConn) close() {
	c.cotp.Close()
}

func (c *serverConn) handle() {
	defer c.server.removeConn(c.id)
	defer c.cotp.Close()

	// 1. Read CN SPDU and send AC SPDU
	//    COTP → Session CN → Presentation CP → ACSE AARQ → MMS Initiate
	rawCN, err := c.cotp.Receive()
	if err != nil {
		return
	}

	cpPDU, err := isosession.ParseConnectSPDU(rawCN)
	if err != nil {
		return
	}

	aarqData, err := isopresentation.ParseConnectAcceptPDU(cpPDU)
	if err != nil {
		return
	}

	mmsInitiateData, err := mms.ParseAARQ(aarqData)
	if err != nil {
		return
	}

	// Accept with default MMS parameters
	_, _, _ = mms.ParseInitiateRequest(mmsInitiateData)
	mmsInitResp := mms.EncodeInitiateResponse()
	aare := mms.BuildAARE(mmsInitResp)
	cpaPDU := isopresentation.BuildConnectAcceptPDU(aare)
	acSPDU := isosession.BuildAcceptSPDU(cpaPDU)
	if err := c.cotp.Send(acSPDU); err != nil {
		return
	}

	// 2. Main MMS service loop
	//    COTP → Session DT → Presentation User Data → MMS PDU
	for {
		raw, err := c.cotp.Receive()
		if err != nil {
			return
		}
		if len(raw) == 0 {
			continue
		}

		// Unwrap Session DT SPDU
		presData, err := isosession.UnwrapDataSPDU(raw)
		if err != nil {
			continue
		}
		// Unwrap Presentation User Data
		data, err := isopresentation.UnwrapUserData(presData)
		if err != nil {
			continue
		}

		respPDU, err := c.dispatchRequest(data)
		if err != nil || respPDU == nil {
			if err != nil {
				errPDU := isosession.WrapDataSPDU(isopresentation.WrapUserData(mms.BuildErrorResponse(0, mms.ErrOther)))
				c.cotp.Send(errPDU)
			}
			return
		}

		// Wrap response in Presentation + Session layers
		wrapped := isosession.WrapDataSPDU(isopresentation.WrapUserData(respPDU))
		if err := c.cotp.Send(wrapped); err != nil {
			return
		}
	}
}

// dispatchRequest routes an MMS request PDU and builds the response.
func (c *serverConn) dispatchRequest(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty PDU")
	}
	pduType := data[0]

	switch pduType {
	case 0xA0: // ConfirmedRequest
		return c.handleConfirmedRequest(data)
	case 0xAB: // ConcludeRequest
		// Orderly shutdown
		resp := []byte{0xAC, 0x00} // ConcludeResponse
		c.cotp.Send(resp)
		return nil, fmt.Errorf("conclude")
	default:
		return nil, fmt.Errorf("unhandled PDU type 0x%02X", pduType)
	}
}

// handleConfirmedRequest parses and dispatches a ConfirmedRequest PDU.
func (c *serverConn) handleConfirmedRequest(data []byte) ([]byte, error) {
	invokeID, svcTag, svcContent, err := mms.ParseConfirmedRequest(data)
	if err != nil {
		return mms.BuildErrorResponse(0, mms.ErrOther), nil
	}

	switch svcTag {
	case mms.SvcRead: // Read
		return c.handleRead(invokeID, svcContent)
	case mms.SvcWrite: // Write
		return c.handleWrite(invokeID, svcContent)
	case mms.SvcGetNameList: // GetNameList
		return c.handleGetNameList(invokeID, svcContent)
	case mms.SvcIdentify: // Identify
		return c.handleIdentify(invokeID)
	case mms.SvcGetVarAccessAttr: // GetVariableAccessAttributes
		return c.handleGetVarAccessAttributes(invokeID, svcContent)
	case mms.SvcGetNamedVarListAttr: // GetNamedVariableListAttributes
		return c.handleGetNamedVarListAttr(invokeID, svcContent)
	default:
		return mms.BuildServiceErrorResponse(invokeID, mms.ErrOther), nil
	}
}

// handleRead serves a Read request.
func (c *serverConn) handleRead(invokeID uint32, content []byte) ([]byte, error) {
	specs, err := mms.ParseReadRequestContent(content)
	if err != nil {
		return mms.BuildErrorResponse(invokeID, mms.ErrInvalidArguments), nil
	}

	var results []*mms.ReadResult
	for _, spec := range specs {
		value, err := c.resolveVariable(spec)
		result := &mms.ReadResult{}
		if err != nil {
			result.IsError = true
			result.Error = mms.DataAccessErrorObjectNonExistent
		} else {
			result.Value = value
		}
		results = append(results, result)
	}

	return mms.BuildReadResponse(invokeID, results), nil
}

// handleWrite serves a Write request.
func (c *serverConn) handleWrite(invokeID uint32, content []byte) ([]byte, error) {
	specs, values, err := mms.ParseWriteRequestContent(content)
	if err != nil {
		return mms.BuildErrorResponse(invokeID, mms.ErrInvalidArguments), nil
	}

	var writeResults []mms.WriteResult
	for i, spec := range specs {
		var val *mms.Value
		if i < len(values) {
			val = values[i]
		}

		wresult := mms.WriteResult{Success: true}
		if err := c.writeVariable(spec, val); err != nil {
			wresult.Success = false
			wresult.Error = mms.DataAccessErrorObjectAccessDenied
		}
		writeResults = append(writeResults, wresult)
	}

	return mms.BuildWriteResponse(invokeID, writeResults), nil
}

// handleGetNameList serves a GetNameList request.
func (c *serverConn) handleGetNameList(invokeID uint32, content []byte) ([]byte, error) {
	req, err := mms.ParseGetNameListRequestContent(content)
	if err != nil {
		return mms.BuildServiceErrorResponse(invokeID, mms.ErrInvalidArguments), nil
	}

	var names []string
	switch req.ObjectClass {
	case mms.ObjectClassType(mms.ObjectClassDomain): // list logical devices (MMS domains)
		for _, ld := range c.server.model.LogicalDevices() {
			names = append(names, c.mmsDomainName(ld))
		}
	case mms.ObjectClassType(mms.ObjectClassNamedVariable): // list named variables within a domain
		names = c.namedVariablesForDomain(req.DomainID)
	case mms.ObjectClassType(mms.ObjectClassNamedVariableList): // list data sets within a domain
		names = c.namedVariableListsForDomain(req.DomainID)
	default:
		// Return empty list for unsupported object classes
	}

	// Apply continueAfter pagination
	if req.ContinueAfter != "" {
		found := false
		for i, n := range names {
			if n == req.ContinueAfter {
				names = names[i+1:]
				found = true
				break
			}
		}
		if !found {
			names = nil
		}
	}

	// Limit response size to avoid exceeding the negotiated MMS PDU size.
	// Each name is encoded as a length-prefixed string; budget ~60 KB conservatively.
	// moreFollows=true signals the client to issue another request with continueAfter.
	const maxResponseBytes = 60000
	moreFollows := false
	budget := maxResponseBytes
	for i, n := range names {
		budget -= 2 + len(n) // 1 byte tag + 1 byte length (short form) + content
		if budget < 0 {
			names = names[:i]
			moreFollows = true
			break
		}
	}

	return mms.BuildGetNameListResponse(invokeID, names, moreFollows), nil
}

// namedVariablesForDomain returns the full MMS named-variable list for a logical device,
// matching the C library behaviour: one entry per level of the hierarchy.
//
//	LN$FC
//	LN$FC$DO
//	LN$FC$DO$DA            (leaf)
//	LN$FC$DO$constructedDA
//	LN$FC$DO$constructedDA$subDA  (leaf)
//
// The list is sorted alphabetically for deterministic continueAfter pagination.
func (c *serverConn) namedVariablesForDomain(domainID string) []string {
	ld := c.findLD(domainID)
	if ld == nil {
		return nil
	}

	// Collect ordered unique FCs for each LN so we emit FC-level entries.
	var names []string
	for _, ln := range ld.LogicalNodes() {
		lnName := ln.Name()

		// Discover which FCs are present and which DOs belong to each FC.
		type doFC struct {
			do *model.DataObject
			fc common.FunctionalConstraint
		}
		fcOrder := []common.FunctionalConstraint{}
		fcSeen := map[common.FunctionalConstraint]bool{}
		fcDOs := map[common.FunctionalConstraint][]*model.DataObject{}

		for _, do := range ln.DataObjects() {
			doFCs := map[common.FunctionalConstraint]bool{}
			for _, child := range do.Children() {
				if da, ok := child.(*model.DataAttribute); ok {
					doFCs[da.FC] = true
				}
			}
			for fc := range doFCs {
				if !fcSeen[fc] {
					fcSeen[fc] = true
					fcOrder = append(fcOrder, fc)
				}
				fcDOs[fc] = append(fcDOs[fc], do)
			}
		}

		// Sort FCs for deterministic output.
		sort.Slice(fcOrder, func(i, j int) bool {
			return fcOrder[i].String() < fcOrder[j].String()
		})

		for _, fc := range fcOrder {
			fcStr := fc.String()
			names = append(names, lnName+"$"+fcStr)

			for _, do := range fcDOs[fc] {
				doPrefix := lnName + "$" + fcStr + "$" + do.Name()
				names = append(names, doPrefix)

				for _, child := range do.Children() {
					da, ok := child.(*model.DataAttribute)
					if !ok || da.FC != fc {
						continue
					}
					collectDANames(doPrefix, da, &names)
				}
			}
		}

		// Add LN$BR as a single top-level named variable if this LN has buffered RCBs.
		// Mirrors C library behaviour: "BR" is one MMS named variable (a STRUCTURE),
		// not a collection of per-field named variables. Sub-elements are accessed via
		// MMS component access (LN$BR$rcbName, LN$BR$rcbName$field), not listed here.
		for _, rcb := range c.server.model.RCBs {
			if rcb.Buffered && rcb.LDInst == ld.Name() && rcb.LNName == lnName {
				names = append(names, lnName+"$BR")
				break
			}
		}
	}

	sort.Strings(names)
	return names
}

// collectDANames emits one entry for the DA itself, then recurses into any sub-DAs.
func collectDANames(prefix string, da *model.DataAttribute, out *[]string) {
	daPath := prefix + "$" + da.Name()
	*out = append(*out, daPath)
	for _, child := range da.Children() {
		if sub, ok := child.(*model.DataAttribute); ok {
			collectDANames(daPath, sub, out)
		}
	}
}

// mmsDomainName returns the MMS domain name for a logical device.
// Per IEC 61850 / C library mms_mapping.c: use LDName if set, otherwise IEDName+LDInst.
func (c *serverConn) mmsDomainName(ld *model.LogicalDevice) string {
	if ld.LDName != "" {
		return ld.LDName
	}
	return c.server.model.Name() + ld.Name()
}

// findLD finds a logical device by its MMS domain name.
func (c *serverConn) findLD(mmsDomain string) *model.LogicalDevice {
	for _, d := range c.server.model.LogicalDevices() {
		if c.mmsDomainName(d) == mmsDomain {
			return d
		}
	}
	return nil
}

// handleGetVarAccessAttributes serves a GetVariableAccessAttributes request.
func (c *serverConn) handleGetVarAccessAttributes(invokeID uint32, content []byte) ([]byte, error) {
	// Parse the variableSpecification: [0] CONSTRUCTED { [1] domainSpecific { visStr domId, visStr itemId } }
	spec, err := mms.ParseGetVarAccessAttributesRequest(content)
	if err != nil || spec.DomainID == "" {
		return mms.BuildServiceErrorResponse(invokeID, mms.ErrOther), nil
	}

	ld := c.findLD(spec.DomainID)
	if ld == nil {
		return mms.BuildServiceErrorResponse(invokeID, mms.ErrAccessObjectNonExistent), nil
	}

	// Look up the named item: just LN name, or LN$FC$DO, or LN$BR[...]
	typeSpec := buildTypeSpecForItem(ld, spec.ItemID, c.server.model.RCBs)
	if typeSpec == nil {
		return mms.BuildServiceErrorResponse(invokeID, mms.ErrAccessObjectNonExistent), nil
	}

	return mms.BuildGetVarAccessAttributesResponse(invokeID, typeSpec), nil
}

// buildTypeSpecForItem builds the MMS TypeSpecification for a variable in a logical device.
// itemID can be just "LN", "LN$BR", "LN$BR$rcbName", or "LN$FC$DO".
func buildTypeSpecForItem(ld *model.LogicalDevice, itemID string, rcbs []*model.ReportControlBlock) []byte {
	// Check for $ separator to find the depth
	dollar := strings.IndexByte(itemID, '$')

	if dollar < 0 {
		// Plain LN name — return the full type spec for the logical node
		for _, ln := range ld.LogicalNodes() {
			if ln.Name() == itemID {
				return buildTypeSpecLN(ln, rcbs, ld.Name())
			}
		}
		return nil
	}

	parts := strings.SplitN(itemID, "$", -1)
	lnName := parts[0]

	// Handle BR (buffered report control block) items.
	if len(parts) >= 2 && parts[1] == "BR" {
		switch len(parts) {
		case 2:
			// LN$BR — structure containing all BRCB instances for this LN
			return buildTypeSpecBRGroup(lnName, ld, rcbs, "")
		case 3:
			// LN$BR$rcbName — type spec for one BRCB instance
			return buildTypeSpecBRCB()
		default:
			// LN$BR$rcbName$fieldName — type spec for one field (not needed by most clients)
			return nil
		}
	}

	// LN$FC$DO[$DA...] — return type spec for the specific FC-grouped data object
	if len(parts) < 3 {
		return nil
	}
	fcStr, doName := parts[1], parts[2]

	for _, ln := range ld.LogicalNodes() {
		if ln.Name() != lnName {
			continue
		}
		for _, do := range ln.DataObjects() {
			if do.Name() != doName {
				continue
			}
			return buildTypeSpecDO(do, fcStr)
		}
	}
	return nil
}

// buildTypeSpecBRGroup returns the TypeSpec for LN$BR — a structure of all BRCB instances.
// ld may be nil; in that case ldInst is used directly for filtering.
func buildTypeSpecBRGroup(lnName string, ld *model.LogicalDevice, rcbs []*model.ReportControlBlock, ldInst string) []byte {
	if ld != nil {
		ldInst = ld.Name()
	}
	var comps []mms.TypeSpecComponent
	for _, rcb := range rcbs {
		if rcb.Buffered && rcb.LDInst == ldInst && rcb.LNName == lnName {
			comps = append(comps, mms.TypeSpecComponent{
				Name:     rcb.Name,
				TypeSpec: buildTypeSpecBRCB(),
			})
		}
	}
	if len(comps) == 0 {
		return nil
	}
	return mms.TypeSpecStructure(comps)
}

// buildTypeSpecBRCB returns the TypeSpec for one BRCB instance.
// Fields match createBufferedReportControlBlock() in reporting.c (13 elements, Ed1).
func buildTypeSpecBRCB() []byte {
	comps := []mms.TypeSpecComponent{
		{Name: "RptID", TypeSpec: mms.TypeSpecVisibleString(129)},
		{Name: "RptEna", TypeSpec: mms.TypeSpecBoolean()},
		{Name: "DatSet", TypeSpec: mms.TypeSpecVisibleString(129)},
		{Name: "ConfRev", TypeSpec: mms.TypeSpecUnsigned(4)},
		{Name: "OptFlds", TypeSpec: mms.TypeSpecBitString(-10)},
		{Name: "BufTm", TypeSpec: mms.TypeSpecUnsigned(4)},
		{Name: "SqNum", TypeSpec: mms.TypeSpecUnsigned(2)},
		{Name: "TrgOps", TypeSpec: mms.TypeSpecBitString(-6)},
		{Name: "IntgPd", TypeSpec: mms.TypeSpecUnsigned(4)},
		{Name: "GI", TypeSpec: mms.TypeSpecBoolean()},
		{Name: "PurgeBuf", TypeSpec: mms.TypeSpecBoolean()},
		{Name: "EntryID", TypeSpec: mms.TypeSpecOctetString(8)},
		{Name: "TimeofEntry", TypeSpec: mms.TypeSpecBinaryTime(6)},
	}
	return mms.TypeSpecStructure(comps)
}

// buildTypeSpecLN builds a TypeSpecification STRUCTURE for a whole logical node.
// Structure: LN → { FC → { DO → { DA... } }, BR → { rcbName → { field... } } }
// rcbs is the full RCB list; used to inject the BR group when the LN has buffered RCBs.
func buildTypeSpecLN(ln *model.LogicalNode, rcbs []*model.ReportControlBlock, ldInst string) []byte {
	// Collect data objects grouped by FC
	fcMap := make(map[string][]string) // FC → list of DO names
	fcOrder := []string{}
	doFCDAs := make(map[string]map[string][]*model.DataAttribute) // do → fc → []DA

	for _, do := range ln.DataObjects() {
		if _, exists := doFCDAs[do.Name()]; !exists {
			doFCDAs[do.Name()] = make(map[string][]*model.DataAttribute)
		}
		for _, child := range do.Children() {
			da, ok := child.(*model.DataAttribute)
			if !ok {
				continue
			}
			fc := da.FC.String()
			if fc == "" || fc == "NONE" {
				continue
			}
			doFCDAs[do.Name()][fc] = append(doFCDAs[do.Name()][fc], da)
			// Track FC order and which DOs belong to each FC
			found := false
			for _, f := range fcOrder {
				if f == fc {
					found = true
					break
				}
			}
			if !found {
				fcOrder = append(fcOrder, fc)
			}
			added := false
			for _, n := range fcMap[fc] {
				if n == do.Name() {
					added = true
					break
				}
			}
			if !added {
				fcMap[fc] = append(fcMap[fc], do.Name())
			}
		}
	}

	sort.Strings(fcOrder)

	// Collect BRCB type spec for this LN (if any).
	brTypeSpec := buildTypeSpecBRGroup(ln.Name(), nil, rcbs, ldInst)

	var lnComponents []mms.TypeSpecComponent
	for _, fc := range fcOrder {
		// Insert BR group before the first FC that sorts after "BR" alphabetically.
		if brTypeSpec != nil && fc > "BR" {
			lnComponents = append(lnComponents, mms.TypeSpecComponent{
				Name:     "BR",
				TypeSpec: brTypeSpec,
			})
			brTypeSpec = nil
		}

		doNames := fcMap[fc]
		sort.Strings(doNames)

		var doComponents []mms.TypeSpecComponent
		for _, doName := range doNames {
			das := doFCDAs[doName][fc]
			doComponents = append(doComponents, mms.TypeSpecComponent{
				Name:     doName,
				TypeSpec: buildTypeSpecDAsAsStructure(das),
			})
		}
		lnComponents = append(lnComponents, mms.TypeSpecComponent{
			Name:     fc,
			TypeSpec: mms.TypeSpecStructure(doComponents),
		})
	}
	// Append BR if it sorts after all FCs or there are no FCs.
	if brTypeSpec != nil {
		lnComponents = append(lnComponents, mms.TypeSpecComponent{
			Name:     "BR",
			TypeSpec: brTypeSpec,
		})
	}
	return mms.TypeSpecStructure(lnComponents)
}

// buildTypeSpecDO builds a TypeSpecification for a specific FC-filtered data object.
func buildTypeSpecDO(do *model.DataObject, fcFilter string) []byte {
	var das []*model.DataAttribute
	for _, child := range do.Children() {
		da, ok := child.(*model.DataAttribute)
		if !ok {
			continue
		}
		if fcFilter == "" || da.FC.String() == fcFilter {
			das = append(das, da)
		}
	}
	return buildTypeSpecDAsAsStructure(das)
}

// buildTypeSpecDAsAsStructure builds a STRUCTURE TypeSpec from a list of data attributes.
func buildTypeSpecDAsAsStructure(das []*model.DataAttribute) []byte {
	var comps []mms.TypeSpecComponent
	for _, da := range das {
		comps = append(comps, mms.TypeSpecComponent{
			Name:     da.Name(),
			TypeSpec: buildTypeSpecDA(da),
		})
	}
	return mms.TypeSpecStructure(comps)
}

// buildTypeSpecDA maps a DataAttribute to its MMS TypeSpecification bytes.
// For CONSTRUCTED DAs (TypeConstructed or any DA with sub-DA children), emits a STRUCTURE.
func buildTypeSpecDA(da *model.DataAttribute) []byte {
	// If the DA has sub-DA children it is CONSTRUCTED regardless of AttrType.
	if subs := da.Children(); len(subs) > 0 {
		var comps []mms.TypeSpecComponent
		for _, child := range subs {
			if sub, ok := child.(*model.DataAttribute); ok {
				comps = append(comps, mms.TypeSpecComponent{
					Name:     sub.Name(),
					TypeSpec: buildTypeSpecDA(sub),
				})
			}
		}
		return mms.TypeSpecStructure(comps)
	}
	switch da.AttrType {
	case common.TypeBoolean:
		return mms.TypeSpecBoolean()
	case common.TypeFLOAT32:
		return mms.TypeSpecFloat32()
	case common.TypeFLOAT64:
		return mms.TypeSpecFloat64()
	case common.TypeINT8:
		return mms.TypeSpecInteger(1)
	case common.TypeINT16:
		return mms.TypeSpecInteger(2)
	case common.TypeINT32:
		return mms.TypeSpecInteger(4)
	case common.TypeINT64:
		return mms.TypeSpecInteger(8)
	case common.TypeINT8U:
		return mms.TypeSpecUnsigned(1)
	case common.TypeINT16U:
		return mms.TypeSpecUnsigned(2)
	case common.TypeINT24U:
		return mms.TypeSpecUnsigned(3)
	case common.TypeINT32U:
		return mms.TypeSpecUnsigned(4)
	case common.TypeQuality:
		return mms.TypeSpecBitString(-13)
	case common.TypeCheck:
		return mms.TypeSpecBitString(-2)
	case common.TypeGenericBitStr:
		return mms.TypeSpecBitString(-32)
	case common.TypeTimestamp:
		return mms.TypeSpecUTCTime()
	case common.TypeVisibleStr32:
		return mms.TypeSpecVisibleString(32)
	case common.TypeVisibleStr64:
		return mms.TypeSpecVisibleString(64)
	case common.TypeVisibleStr65:
		return mms.TypeSpecVisibleString(65)
	case common.TypeVisibleStr129:
		return mms.TypeSpecVisibleString(129)
	case common.TypeVisibleStr255:
		return mms.TypeSpecVisibleString(255)
	case common.TypeOctetString64:
		return mms.TypeSpecOctetString(64)
	case common.TypeOctetString6:
		return mms.TypeSpecOctetString(6)
	case common.TypeOctetString8:
		return mms.TypeSpecOctetString(8)
	default:
		return mms.TypeSpecVisibleString(255)
	}
}

// namedVariableListsForDomain returns data set names for a logical device domain.
// Member references store only the LDInst; expand to full MMS domain name for comparison.
func (c *serverConn) namedVariableListsForDomain(domainID string) []string {
	var names []string
	for _, ds := range c.server.model.DataSets {
		if domainID == "" {
			names = append(names, ds.Name)
			continue
		}
		for _, m := range ds.Members {
			parts := splitRef(m.Reference)
			if len(parts) > 0 && c.ldInstToMmsDomain(parts[0]) == domainID {
				names = append(names, ds.Name)
				break
			}
		}
	}
	return names
}

// ldInstToMmsDomain converts a bare LDInst name to its MMS domain name by
// finding the matching logical device and prepending the IED name.
// If no LD is found (e.g. cross-IED reference), the ldInst is returned unchanged.
func (c *serverConn) ldInstToMmsDomain(ldInst string) string {
	for _, ld := range c.server.model.LogicalDevices() {
		if ld.Name() == ldInst {
			return c.mmsDomainName(ld)
		}
	}
	return ldInst
}

// collectFCs returns the unique functional constraint strings for all data attributes in a DO.
func collectFCs(do *model.DataObject) []string {
	seen := make(map[string]bool)
	var fcs []string
	for _, child := range do.Children() {
		if da, ok := child.(*model.DataAttribute); ok {
			s := da.FC.String()
			if s != "" && s != "NONE" && !seen[s] {
				seen[s] = true
				fcs = append(fcs, s)
			}
		}
	}
	return fcs
}

// splitRef splits a reference string like "domainID/LN.DO" by "/".
func splitRef(ref string) []string {
	for i, c := range ref {
		if c == '/' {
			return []string{ref[:i], ref[i+1:]}
		}
	}
	return []string{ref}
}

// handleIdentify serves an Identify request, returning server information.
func (c *serverConn) handleIdentify(invokeID uint32) ([]byte, error) {
	return mms.BuildIdentifyResponse(invokeID, "libiec61850-Go", "1.0.0", "IEC 61850 Server"), nil
}

// resolveVariable finds the value of a variable specification in the model.
// itemID formats: "LN", "LN$FC", "LN$FC$DO", "LN$FC$DO$DA"
func (c *serverConn) resolveVariable(spec mms.VariableSpecification) (*mms.Value, error) {
	parts := strings.SplitN(spec.ItemID, "$", -1)

	ld := c.findLD(spec.DomainID)
	if ld == nil {
		return nil, fmt.Errorf("domain not found: %s", spec.DomainID)
	}

	// Find the logical node
	var ln *model.LogicalNode
	for _, l := range ld.LogicalNodes() {
		if l.Name() == parts[0] {
			ln = l
			break
		}
	}
	if ln == nil {
		return nil, fmt.Errorf("node not found: %s/%s", spec.DomainID, spec.ItemID)
	}

	// Handle BR (buffered RCB) items: LN$BR, LN$BR$rcbName, LN$BR$rcbName$fieldName
	if len(parts) >= 2 && parts[1] == "BR" {
		return c.resolveBRCBVariable(parts, ld.Name())
	}

	switch len(parts) {
	case 1:
		// "LN" — return all data grouped by FC then DO, with BR group if present
		return c.buildStructureFromLNWithBR(ln, ld.Name()), nil
	case 2:
		// "LN$FC" — return structure of DOs filtered by FC
		return buildStructureByFC(ln, parts[1])
	default:
		// "LN$FC$DO[$DA...]" — strip FC (parts[1]) and look up by DO[.DA...]
		dotPath := spec.DomainID + "/" + parts[0] + "." + strings.Join(parts[2:], ".")
		node := c.server.model.FindNode(dotPath)
		if node == nil {
			return nil, fmt.Errorf("not found: %s", dotPath)
		}
		switch n := node.(type) {
		case *model.DataAttribute:
			if n.Value == nil {
				return mms.NewDataAccessError(mms.DataAccessErrorTemporarilyUnavailable), nil
			}
			return n.Value, nil
		case *model.DataObject:
			return buildStructureFromDO(n), nil
		default:
			return nil, fmt.Errorf("unsupported node type for %s", dotPath)
		}
	}
}

// resolveBRCBVariable resolves a BRCB variable.
// parts[0]=LNName, parts[1]="BR", parts[2]=rcbName (opt), parts[3]=fieldName (opt).
// ldInst is the logical device instance name for filtering.
func (c *serverConn) resolveBRCBVariable(parts []string, ldInst string) (*mms.Value, error) {
	lnName := parts[0]
	switch len(parts) {
	case 2:
		// LN$BR — return structure of all BRCB instances for this LN
		var members []*mms.Value
		for _, rcb := range c.server.model.RCBs {
			if rcb.Buffered && rcb.LDInst == ldInst && rcb.LNName == lnName {
				state := c.server.brcbValues[rcb.Name]
				if state == nil {
					continue
				}
				members = append(members, mms.NewStructure(state.asValues()))
			}
		}
		if len(members) == 0 {
			return nil, fmt.Errorf("no BRCB for LN %s", lnName)
		}
		return mms.NewStructure(members), nil
	case 3:
		// LN$BR$rcbName — return one BRCB instance as a structure
		rcbName := parts[2]
		state := c.server.brcbValues[rcbName]
		if state == nil {
			return nil, fmt.Errorf("BRCB not found: %s", rcbName)
		}
		return mms.NewStructure(state.asValues()), nil
	case 4:
		// LN$BR$rcbName$fieldName
		rcbName, fieldName := parts[2], parts[3]
		state := c.server.brcbValues[rcbName]
		if state == nil {
			return nil, fmt.Errorf("BRCB not found: %s", rcbName)
		}
		v := state.fieldValue(fieldName)
		if v == nil {
			return nil, fmt.Errorf("BRCB field not found: %s", fieldName)
		}
		return v, nil
	}
	return nil, fmt.Errorf("invalid BRCB path")
}

// buildStructureByFC returns a STRUCTURE value for all DataObjects with the given FC.
// DOs are sorted alphabetically to match the order used in buildTypeSpecLN, so the
// returned value structure aligns with the type spec that clients received via GVAA.
func buildStructureByFC(ln *model.LogicalNode, fc string) (*mms.Value, error) {
	dos := ln.DataObjects()
	sorted := make([]*model.DataObject, len(dos))
	copy(sorted, dos)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name() < sorted[j].Name() })

	var members []*mms.Value
	found := false
	for _, do := range sorted {
		var dMembers []*mms.Value
		for _, child := range do.Children() {
			da, ok := child.(*model.DataAttribute)
			if !ok || da.FC.String() != fc {
				continue
			}
			dMembers = append(dMembers, buildValueFromDA(da))
			found = true
		}
		if len(dMembers) > 0 {
			members = append(members, mms.NewStructure(dMembers))
		}
	}
	if !found {
		return nil, fmt.Errorf("no %s data in %s", fc, ln.Name())
	}
	return mms.NewStructure(members), nil
}

// handleGetNamedVarListAttr serves a GetNamedVariableListAttributes request.
// It returns the list of variable specifications (members) for a named variable list (dataset).
func (c *serverConn) handleGetNamedVarListAttr(invokeID uint32, content []byte) ([]byte, error) {
	domainID, itemID, err := mms.ParseGetNamedVarListAttrRequest(content)
	if err != nil || domainID == "" || itemID == "" {
		return mms.BuildServiceErrorResponse(invokeID, mms.ErrAccessObjectNonExistent), nil
	}

	// Find the dataset
	var ds *model.DataSet
	for _, d := range c.server.model.DataSets {
		if d.Name == itemID {
			ds = d
			break
		}
	}
	if ds == nil {
		return mms.BuildServiceErrorResponse(invokeID, mms.ErrAccessObjectNonExistent), nil
	}

	// Build member variable specs.
	// Reference format: "Device1/LLN0.Mod.q" + FC_ST → domain="SampleIEDDevice1", itemId="LLN0$ST$Mod$q".
	// The reference stores the bare LDInst; expand it to the full MMS domain name.
	var members []mms.VariableSpecification
	for _, m := range ds.Members {
		slash := strings.IndexByte(m.Reference, '/')
		if slash < 0 {
			continue
		}
		ldInst := m.Reference[:slash]
		rest := m.Reference[slash+1:] // "LLN0.Mod.q"
		parts := strings.Split(rest, ".")
		// insert FC after LN name
		mItemID := parts[0] + "$" + m.FC.String()
		if len(parts) > 1 {
			mItemID += "$" + strings.Join(parts[1:], "$")
		}
		members = append(members, mms.VariableSpecification{
			DomainID: c.ldInstToMmsDomain(ldInst),
			ItemID:   mItemID,
		})
	}

	return mms.BuildGetNamedVarListAttrResponse(invokeID, false, members), nil
}

// writeVariable writes a value to a named variable in the model.
func (c *serverConn) writeVariable(spec mms.VariableSpecification, value *mms.Value) error {
	parts := strings.SplitN(spec.ItemID, "$", -1)

	// Handle BRCB writes: LN$BR$rcbName$fieldName
	if len(parts) == 4 && parts[1] == "BR" {
		rcbName, fieldName := parts[2], parts[3]
		c.server.mu.Lock()
		state := c.server.brcbValues[rcbName]
		c.server.mu.Unlock()
		if state == nil {
			return fmt.Errorf("BRCB not found: %s", rcbName)
		}
		if !state.setFieldValue(fieldName, value) {
			return fmt.Errorf("BRCB field not found: %s", fieldName)
		}
		return nil
	}

	node := c.server.model.FindNode(spec.DomainID + "/" + dotifyItemID(spec.ItemID))
	if node == nil {
		return fmt.Errorf("not found: %s/%s", spec.DomainID, spec.ItemID)
	}

	da, ok := node.(*model.DataAttribute)
	if !ok {
		return fmt.Errorf("node is not a data attribute")
	}

	// Check access handler
	if c.server.writeHandler != nil {
		if err := c.server.writeHandler(da, value, c.cotp.RemoteAddr()); err != nil {
			return err
		}
	}

	da.Value = value
	return nil
}

// dotifyItemID converts an MMS item ID (LN$FC$DO$DA) to dot notation (LN.DO.DA).
func dotifyItemID(itemID string) string {
	out := make([]byte, len(itemID))
	for i := 0; i < len(itemID); i++ {
		if itemID[i] == '$' {
			// Skip FC field: if the segment looks like a 2-char FC abbreviation, skip it
			out[i] = '.'
		} else {
			out[i] = itemID[i]
		}
	}
	return string(out)
}

// buildStructureFromDO builds an MMS STRUCTURE value from a DataObject's attributes.
func buildStructureFromDO(do *model.DataObject) *mms.Value {
	var members []*mms.Value
	for _, child := range do.Children() {
		switch n := child.(type) {
		case *model.DataAttribute:
			members = append(members, buildValueFromDA(n))
		case *model.DataObject:
			members = append(members, buildStructureFromDO(n))
		}
	}
	return mms.NewStructure(members)
}

// buildValueFromDA returns the MMS value for a DataAttribute.
// CONSTRUCTED DAs (those with sub-DA children) yield a nested STRUCTURE.
func buildValueFromDA(da *model.DataAttribute) *mms.Value {
	subs := da.Children()
	if len(subs) > 0 {
		var members []*mms.Value
		for _, child := range subs {
			if sub, ok := child.(*model.DataAttribute); ok {
				members = append(members, buildValueFromDA(sub))
			}
		}
		return mms.NewStructure(members)
	}
	if da.Value != nil {
		return da.Value
	}
	return mms.NewDataAccessError(mms.DataAccessErrorTemporarilyUnavailable)
}

// buildStructureFromLN builds an MMS STRUCTURE value from all DOs in a LogicalNode.
func buildStructureFromLN(ln *model.LogicalNode) *mms.Value {
	var members []*mms.Value
	for _, child := range ln.Children() {
		if do, ok := child.(*model.DataObject); ok {
			members = append(members, buildStructureFromDO(do))
		}
	}
	return mms.NewStructure(members)
}

// buildStructureFromLNWithBR builds the whole-LN value in the same order as buildTypeSpecLN:
// FC groups sorted alphabetically, with BR inserted in its alphabetical position.
func (c *serverConn) buildStructureFromLNWithBR(ln *model.LogicalNode, ldInst string) *mms.Value {
	// Collect FC groups (same logic as buildTypeSpecLN)
	fcDOs := make(map[string][]*model.DataObject) // FC → DOs (sorted)
	fcOrder := []string{}

	for _, do := range ln.DataObjects() {
		doFCs := map[string]bool{}
		for _, child := range do.Children() {
			if da, ok := child.(*model.DataAttribute); ok {
				doFCs[da.FC.String()] = true
			}
		}
		for fc := range doFCs {
			if fc == "" || fc == "NONE" {
				continue
			}
			found := false
			for _, f := range fcOrder {
				if f == fc {
					found = true
					break
				}
			}
			if !found {
				fcOrder = append(fcOrder, fc)
			}
			fcDOs[fc] = append(fcDOs[fc], do)
		}
	}
	sort.Strings(fcOrder)

	// Build BR group value if present (mirrors buildTypeSpecBRGroup logic)
	var brValue *mms.Value
	for _, rcb := range c.server.model.RCBs {
		if rcb.Buffered && rcb.LDInst == ldInst && rcb.LNName == ln.Name() {
			state := c.server.brcbValues[rcb.Name]
			if state != nil {
				brValue = mms.NewStructure([]*mms.Value{mms.NewStructure(state.asValues())})
			}
			break
		}
	}

	var members []*mms.Value
	brInserted := false
	for _, fc := range fcOrder {
		if brValue != nil && !brInserted && fc > "BR" {
			members = append(members, brValue)
			brInserted = true
		}
		// Build FC group value (DOs sorted alphabetically)
		dos := fcDOs[fc]
		sort.Slice(dos, func(i, j int) bool { return dos[i].Name() < dos[j].Name() })
		var doMembers []*mms.Value
		for _, do := range dos {
			var dMembers []*mms.Value
			for _, child := range do.Children() {
				if da, ok := child.(*model.DataAttribute); ok && da.FC.String() == fc {
					dMembers = append(dMembers, buildValueFromDA(da))
				}
			}
			doMembers = append(doMembers, mms.NewStructure(dMembers))
		}
		members = append(members, mms.NewStructure(doMembers))
	}
	if brValue != nil && !brInserted {
		members = append(members, brValue)
	}
	return mms.NewStructure(members)
}
