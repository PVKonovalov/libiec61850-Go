/*
 *  model.go
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

// Package model defines the IEC 61850 data model hierarchy:
//
//	IedModel
//	└── LogicalDevice (LD)
//	    └── LogicalNode (LN)
//	        └── DataObject (DO)
//	            └── DataAttribute (DA)
//
// The model is used both by servers (to define their information model)
// and by clients (to navigate the discovered server structure).
package model

import (
	"fmt"
	"strings"

	"github.com/PVKonovalov/libiec61850-Go/pkg/iec61850/common"
	"github.com/PVKonovalov/libiec61850-Go/pkg/mms"
)

// NodeType identifies the type of model node.
type NodeType int

const (
	NodeTypeLogicalDevice NodeType = 0
	NodeTypeLogicalNode   NodeType = 1
	NodeTypeDataObject    NodeType = 2
	NodeTypeDataAttribute NodeType = 3
)

// Node is the base interface implemented by all model elements.
type Node interface {
	Name() string
	NodeType() NodeType
	Parent() Node
	Children() []Node
	Path() string // full object reference path
}

// baseNode holds common fields for all model nodes.
type baseNode struct {
	name     string
	nodeType NodeType
	parent   Node
	children []Node
}

func (n *baseNode) Name() string       { return n.name }
func (n *baseNode) NodeType() NodeType { return n.nodeType }
func (n *baseNode) Parent() Node       { return n.parent }
func (n *baseNode) Children() []Node   { return n.children }

func (n *baseNode) addChild(child Node) {
	n.children = append(n.children, child)
}

// IedModel is the root of the IEC 61850 data model.
// It holds one or more LogicalDevices and associated data sets.
type IedModel struct {
	baseNode
	DataSets []*DataSet
	RCBs     []*ReportControlBlock
	GSECBs   []*GSEControlBlock
	SVCBs    []*SVControlBlock
}

// NewIedModel creates a new empty IED model with the given name.
func NewIedModel(name string) *IedModel {
	m := &IedModel{}
	m.name = name
	m.nodeType = NodeTypeLogicalDevice
	return m
}

// Path returns the IED model name.
func (m *IedModel) Path() string { return m.name }

// AddLogicalDevice adds a logical device to the IED model.
func (m *IedModel) AddLogicalDevice(ld *LogicalDevice) {
	ld.parent = m
	m.addChild(ld)
}

// LogicalDevices returns all logical devices in the model.
func (m *IedModel) LogicalDevices() []*LogicalDevice {
	var lds []*LogicalDevice
	for _, c := range m.children {
		if ld, ok := c.(*LogicalDevice); ok {
			lds = append(lds, ld)
		}
	}
	return lds
}

// FindNode finds a node by its object reference path.
func (m *IedModel) FindNode(objectRef string) Node {
	// Parse "LD/LN.DO.DA" or "LD/LN$FC$DO$DA"
	slashIdx := strings.IndexByte(objectRef, '/')
	if slashIdx < 0 {
		return nil
	}
	ldName := objectRef[:slashIdx]
	rest := objectRef[slashIdx+1:]

	for _, c := range m.children {
		if ld, ok := c.(*LogicalDevice); ok && ld.name == ldName {
			return ld.findInLD(rest)
		}
	}
	return nil
}

// LogicalDevice (LD) is a virtual device hosted by an IED.
// It groups logical nodes that implement specific functions.
type LogicalDevice struct {
	baseNode
	LDName string // optional functional naming (ldName attribute)
}

// NewLogicalDevice creates a new logical device.
func NewLogicalDevice(name string, parent *IedModel) *LogicalDevice {
	ld := &LogicalDevice{}
	ld.name = name
	ld.nodeType = NodeTypeLogicalDevice
	ld.parent = parent
	if parent != nil {
		parent.addChild(ld)
	}
	return ld
}

// Path returns the logical device path (just its name in IEC 61850).
func (ld *LogicalDevice) Path() string {
	return ld.name
}

// AddLogicalNode adds a logical node to this device.
func (ld *LogicalDevice) AddLogicalNode(ln *LogicalNode) {
	ln.parent = ld
	ld.addChild(ln)
}

// LogicalNodes returns all logical nodes in this device.
func (ld *LogicalDevice) LogicalNodes() []*LogicalNode {
	var lns []*LogicalNode
	for _, c := range ld.children {
		if ln, ok := c.(*LogicalNode); ok {
			lns = append(lns, ln)
		}
	}
	return lns
}

func (ld *LogicalDevice) findInLD(rest string) Node {
	// rest = "LN.DO.DA" or "LN$FC$DO$DA"
	// Normalize $-separated form to dot-separated for lookup
	normalized := strings.ReplaceAll(rest, "$", ".")
	parts := strings.SplitN(normalized, ".", 4)
	if len(parts) == 0 {
		return nil
	}
	lnName := parts[0]
	for _, c := range ld.children {
		ln, ok := c.(*LogicalNode)
		if !ok || ln.name != lnName {
			continue
		}
		if len(parts) == 1 {
			return ln
		}
		return ln.findInLN(parts[1:])
	}
	return nil
}

// LogicalNode (LN) implements a specific function within a logical device.
// Logical nodes are named with a class name + instance number (e.g., "XCBR1").
type LogicalNode struct {
	baseNode
}

// NewLogicalNode creates a new logical node and optionally adds it to a device.
func NewLogicalNode(name string, parent *LogicalDevice) *LogicalNode {
	ln := &LogicalNode{}
	ln.name = name
	ln.nodeType = NodeTypeLogicalNode
	ln.parent = parent
	if parent != nil {
		parent.addChild(ln)
	}
	return ln
}

// Path returns the full path of this logical node (LD/LN).
func (ln *LogicalNode) Path() string {
	if ln.parent == nil {
		return ln.name
	}
	return ln.parent.Path() + "/" + ln.name
}

// AddDataObject adds a data object to this logical node.
func (ln *LogicalNode) AddDataObject(do *DataObject) {
	do.parent = ln
	ln.addChild(do)
}

// DataObjects returns all data objects in this logical node.
func (ln *LogicalNode) DataObjects() []*DataObject {
	var dos []*DataObject
	for _, c := range ln.children {
		if do, ok := c.(*DataObject); ok {
			dos = append(dos, do)
		}
	}
	return dos
}

func (ln *LogicalNode) findInLN(parts []string) Node {
	if len(parts) == 0 {
		return ln
	}
	doName := parts[0]
	for _, c := range ln.children {
		do, ok := c.(*DataObject)
		if !ok || do.name != doName {
			continue
		}
		if len(parts) == 1 {
			return do
		}
		return do.findInDO(parts[1:])
	}
	return nil
}

// DataObject (DO) holds data attributes organized by functional constraint.
type DataObject struct {
	baseNode
	ElementCount int // > 0 if this is an array
	ArrayIndex   int // > -1 if this is an array element
}

// NewDataObject creates a new data object and optionally adds it to a logical node.
func NewDataObject(name string, parent *LogicalNode) *DataObject {
	do := &DataObject{ArrayIndex: -1}
	do.name = name
	do.nodeType = NodeTypeDataObject
	do.parent = parent
	if parent != nil {
		parent.addChild(do)
	}
	return do
}

// Path returns the full path of this data object.
func (do *DataObject) Path() string {
	if do.parent == nil {
		return do.name
	}
	return do.parent.Path() + "." + do.name
}

// AddDataAttribute adds a data attribute to this data object.
func (do *DataObject) AddDataAttribute(da *DataAttribute) {
	da.parent = do
	do.addChild(da)
}

// DataAttributes returns all data attributes in this data object.
func (do *DataObject) DataAttributes() []*DataAttribute {
	var das []*DataAttribute
	for _, c := range do.children {
		if da, ok := c.(*DataAttribute); ok {
			das = append(das, da)
		}
	}
	return das
}

func (do *DataObject) findInDO(parts []string) Node {
	if len(parts) == 0 {
		return do
	}
	daName := parts[0]
	for _, c := range do.children {
		da, ok := c.(*DataAttribute)
		if !ok || da.name != daName {
			continue
		}
		if len(parts) == 1 {
			return da
		}
		return da.findInDA(parts[1:])
	}
	return nil
}

// DataAttribute (DA) holds the actual value of an IEC 61850 attribute.
type DataAttribute struct {
	baseNode
	ElementCount   int
	ArrayIndex     int
	FC             common.FunctionalConstraint
	AttrType       common.DataAttributeType
	TriggerOptions common.TriggerOption
	Value          *mms.Value // current value (nil until set)
}

// NewDataAttribute creates a new data attribute.
func NewDataAttribute(name string, fc common.FunctionalConstraint, attrType common.DataAttributeType, parent *DataObject) *DataAttribute {
	da := &DataAttribute{
		ArrayIndex: -1,
		FC:         fc,
		AttrType:   attrType,
	}
	da.name = name
	da.nodeType = NodeTypeDataAttribute
	da.parent = parent
	if parent != nil {
		parent.addChild(da)
	}
	return da
}

// NewSubDataAttribute creates a sub data attribute (child of a CONSTRUCTED DataAttribute).
func NewSubDataAttribute(name string, fc common.FunctionalConstraint, attrType common.DataAttributeType, parent *DataAttribute) *DataAttribute {
	da := &DataAttribute{
		ArrayIndex: -1,
		FC:         fc,
		AttrType:   attrType,
	}
	da.name = name
	da.nodeType = NodeTypeDataAttribute
	da.parent = parent
	if parent != nil {
		parent.addChild(da)
	}
	return da
}

// Path returns the full path of this data attribute, including FC.
func (da *DataAttribute) Path() string {
	if da.parent == nil {
		return da.name
	}
	return da.parent.Path() + "." + da.name
}

// PathWithFC returns the path with the functional constraint appended (e.g., "LD/LN.DO.DA$ST").
func (da *DataAttribute) PathWithFC() string {
	return da.Path() + "$" + da.FC.String()
}

func (da *DataAttribute) findInDA(parts []string) Node {
	if len(parts) == 0 {
		return da
	}
	childName := parts[0]
	for _, c := range da.children {
		child, ok := c.(*DataAttribute)
		if !ok || child.name != childName {
			continue
		}
		if len(parts) == 1 {
			return child
		}
		return child.findInDA(parts[1:])
	}
	return nil
}

// DataSet defines a named group of data set members (variable references).
// Data sets are the foundation of reporting and GOOSE.
type DataSet struct {
	Name    string
	Members []DataSetMember
}

// DataSetMember references one data attribute in a data set.
type DataSetMember struct {
	Reference string // IEC 61850 object reference
	FC        common.FunctionalConstraint
}

// ReportControlBlock defines the parameters of a report control block (RCB).
type ReportControlBlock struct {
	Name         string
	DataSetRef   string
	Buffered     bool // true = BRCB, false = URCB
	RptID        string
	ConfRev      uint32
	OptFields    common.ReportOption
	BufTime      uint32
	TrgOps       common.TriggerOption
	IntgPd       uint32
	Indexed      bool
	MaxInstances int
}

// GSEControlBlock defines parameters for a GOOSE control block.
type GSEControlBlock struct {
	Name       string
	DataSetRef string
	AppID      string
	PhyComAddr common.PhyComAddress
	FixedOffs  int32
	MinTime    int32
	MaxTime    int32
	Buffered   bool
}

// SVControlBlock defines parameters for a Sampled Values control block.
type SVControlBlock struct {
	Name       string
	DataSetRef string
	SVCBRef    string
	SmpRate    uint16
	OptFields  uint8
	PhyComAddr common.PhyComAddress
	Multicast  bool
}

// MMSVariableName converts an IEC 61850 object reference to the MMS variable
// name used on the wire (domain-specific form).
//
// IEC 61850-8-1 defines the mapping:
//   - Domain = logical device name
//   - Item = LNName$FC$DOName[$DAName[...]]
func MMSVariableName(objectRef string, fc common.FunctionalConstraint) (domainID, itemID string, err error) {
	slashIdx := strings.IndexByte(objectRef, '/')
	if slashIdx < 0 {
		return "", "", fmt.Errorf("model: missing '/' in reference %q", objectRef)
	}
	domainID = objectRef[:slashIdx]
	rest := objectRef[slashIdx+1:]

	// Normalize: replace existing $FC$ with dots so we can re-insert the correct FC
	rest = strings.ReplaceAll(rest, "$", ".")
	parts := strings.SplitN(rest, ".", 2)
	if len(parts) < 2 {
		return "", "", fmt.Errorf("model: expected LN.DO in %q", rest)
	}

	lnName := parts[0]
	doDA := parts[1]

	if fc == common.FC_NONE || fc == common.FC_ALL {
		itemID = lnName + "." + doDA
	} else {
		// Replace first dot with $FC$
		dotIdx := strings.IndexByte(doDA, '.')
		if dotIdx < 0 {
			itemID = lnName + "$" + fc.String() + "$" + doDA
		} else {
			itemID = lnName + "$" + fc.String() + "$" + doDA[:dotIdx] + "$" + doDA[dotIdx+1:]
		}
	}
	return
}
