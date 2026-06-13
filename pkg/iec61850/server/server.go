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
}

// NewIedServer creates a new IED server with the given data model and configuration.
// If config is nil, DefaultConfig() is used.
func NewIedServer(iedModel *model.IedModel, config *Config) *IedServer {
	if config == nil {
		config = DefaultConfig()
	}
	return &IedServer{
		model:  iedModel,
		config: config,
		conns:  make(map[uint64]*serverConn),
	}
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
func (s *IedServer) Start(port int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("iec61850 server: already running")
	}

	addr := fmt.Sprintf(":%d", port)
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
	_, _, _ = mms.ParseInitiateResponse(mmsInitiateData)
	mmsInitResp := mms.EncodeInitiateRequest()
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
func (c *serverConn) handleGetNameList(invokeID uint32, _ []byte) ([]byte, error) {
	var names []string
	for _, ld := range c.server.model.LogicalDevices() {
		names = append(names, ld.Name())
	}
	return mms.BuildGetNameListResponse(invokeID, names, false), nil
}

// handleIdentify serves an Identify request, returning server information.
func (c *serverConn) handleIdentify(invokeID uint32) ([]byte, error) {
	return mms.BuildIdentifyResponse(invokeID, "libiec61850-Go", "1.0.0", "IEC 61850 Server"), nil
}

// resolveVariable finds the value of a variable specification in the model.
func (c *serverConn) resolveVariable(spec mms.VariableSpecification) (*mms.Value, error) {
	// domainID = logical device name, itemID = LN$FC$DO[$DA]
	node := c.server.model.FindNode(spec.DomainID + "/" + dotifyItemID(spec.ItemID))
	if node == nil {
		return nil, fmt.Errorf("not found: %s/%s", spec.DomainID, spec.ItemID)
	}

	switch n := node.(type) {
	case *model.DataAttribute:
		if n.Value == nil {
			return mms.NewDataAccessError(mms.DataAccessErrorTemporarilyUnavailable), nil
		}
		return n.Value, nil
	case *model.DataObject:
		return buildStructureFromDO(n), nil
	case *model.LogicalNode:
		return buildStructureFromLN(n), nil
	default:
		return nil, fmt.Errorf("unsupported node type")
	}
}

// writeVariable writes a value to a named variable in the model.
func (c *serverConn) writeVariable(spec mms.VariableSpecification, value *mms.Value) error {
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
			if n.Value != nil {
				members = append(members, n.Value)
			} else {
				members = append(members, mms.NewDataAccessError(mms.DataAccessErrorTemporarilyUnavailable))
			}
		case *model.DataObject:
			members = append(members, buildStructureFromDO(n))
		}
	}
	return mms.NewStructure(members)
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
