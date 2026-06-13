# libiec61850-Go

A native Go implementation of the IEC 61850 communication standard for
power system automation (MMS client/server, GOOSE, Sampled Values).

This library implements the IEC 61850 protocol stack from scratch in Go —
no CGo, no bindings. It is a Golang port and re-implementation of
[libiec61850](https://github.com/mz-automation/libiec61850).

---

## Protocol Stack

```
Application:  IEC 61850 ACSI (client & server)
              GOOSE (raw Ethernet, IEC 61850-8-1)
              Sampled Values (raw Ethernet, IEC 61850-9-2)
              ─────────────────────────────────────
Protocol:     MMS (ISO 9506, Manufacturing Message Specification)
              ACSE (ISO 8649/8650, Association Control)
              ─────────────────────────────────────
Transport:    ISO COTP (ISO 8073, RFC 1006 over TCP)
              TCP/IP
```

---

## Package Layout

| Package               | Description                                                           |
|-----------------------|-----------------------------------------------------------------------|
| `pkg/asn1ber`         | ASN.1 Basic Encoding Rules (BER) encoder/decoder                      |
| `pkg/cotp`            | ISO 8073 Connection-Oriented Transport Protocol over RFC 1006/TCP     |
| `pkg/acse`            | ACSE (Association Control Service Element) – connection establishment |
| `pkg/mms`             | MMS types, values, PDU encoding/decoding                              |
| `pkg/iec61850/common` | IEC 61850 types: functional constraints, quality, control models      |
| `pkg/iec61850/model`  | IEC 61850 data model (IedModel → LD → LN → DO → DA)                   |
| `pkg/iec61850/client` | IEC 61850 MMS client                                                  |
| `pkg/iec61850/server` | IEC 61850 MMS server                                                  |
| `pkg/goose`           | GOOSE publisher and subscriber (raw Ethernet)                         |
| `pkg/sv`              | Sampled Values publisher and subscriber (raw Ethernet)                |

---

## Requirements

- Go 1.26+
- Linux with `CAP_NET_RAW` (or root) for GOOSE/SV raw Ethernet sockets
- macOS/Windows: GOOSE/SV stubs are included but raw socket support
  requires platform-specific implementation (BPF on macOS, WinPcap on Windows)

---

## IEC 61850 Client

### Connecting to an IED

```go
import (
"github.com/PVKonovalov/libiec61850-Go/pkg/iec61850/client"
"github.com/PVKonovalov/libiec61850-Go/pkg/iec61850/common"
"github.com/PVKonovalov/libiec61850-Go/pkg/mms"
)

// Connect (performs COTP + ACSE + MMS handshake automatically)
conn, err := client.Dial("192.168.1.100:102")
if err != nil {
log.Fatal(err)
}
defer conn.Close()
```

### Reading a Data Attribute

Object references use the IEC 61850 path format:
`LogicalDevice/LogicalNode.DataObject[.DataAttribute][$FunctionalConstraint]`

```go
// Read a float measurand (FC=MX)
value, err := conn.ReadObject(
"simpleIOGenericIO/GGIO1.AnIn1.mag.f",
common.FC_MX,
)
if err != nil {
log.Fatal(err)
}
if value.Type() == mms.TypeFloat {
fmt.Printf("AnIn1 magnitude: %f\n", value.GetFloat32())
}
```

### Writing a Data Attribute

```go
// Write a visible string (FC=DC for description/configuration)
err = conn.WriteObject(
"simpleIOGenericIO/GGIO1.NamPlt.vendor",
common.FC_DC,
mms.NewVisibleString("My Application"),
)
```

### Reading a Data Set

```go
dataSet, err := conn.ReadDataSetValues("simpleIOGenericIO/LLN0.Events", nil)
if err != nil {
log.Fatal(err)
}
values := dataSet.GetDataSetValues() // *mms.Value of TypeStructure
for i := 0; i < values.Size(); i++ {
fmt.Printf("  member[%d] = %s\n", i, values.GetElement(i))
}
```

### Reporting (RCB subscription)

```go
// 1. Read the Report Control Block configuration
rcb, err := conn.GetRCBValues("simpleIOGenericIO/LLN0.RP.EventsRCB01")
if err != nil {
log.Fatal(err)
}

// 2. Install a report handler (keyed by the RCB's report ID)
conn.InstallReportHandler(
"simpleIOGenericIO/LLN0.RP.EventsRCB01",
rcb.RptID,
func (report *client.Report) {
fmt.Printf("report received: %s\n", report.RCBReference)
vals := report.DataSetValues
for i := 0; i < vals.Size(); i++ {
reason := report.ReasonForInclusion[i]
if reason != common.ReasonNotIncluded {
fmt.Printf("  [%d] %s (reason=%d)\n", i, vals.GetElement(i), reason)
}
}
},
)

// 3. Enable reporting with trigger options
rcb.RptEna = true
rcb.TrgOps = common.TriggerDataChanged | common.TriggerIntegrity | common.TriggerGI
rcb.IntgPd = 5000 // integrity period in ms

err = conn.SetRCBValues(rcb,
client.RCBElementRptEna | client.RCBElementTrgOps | client.RCBElementIntgPd,
true,
)

// 4. Trigger a General Interrogation to receive all current values
rcb.GI = true
conn.SetRCBValues(rcb, client.RCBElementGI, true)
```

### Server Directory Browsing

```go
// List logical devices on the server
devices, err := conn.GetServerDirectory()
// e.g.: ["simpleIO", "MeasDevice"]

// List logical nodes in a device
nodes, err := conn.GetLogicalDeviceDirectory("simpleIO")
// e.g.: ["LLN0", "GGIO1"]
```

---

## IEC 61850 Server

### Building a Data Model

```go
import (
"github.com/PVKonovalov/libiec61850-Go/pkg/iec61850/common"
imodel "github.com/PVKonovalov/libiec61850-Go/pkg/iec61850/model"
"github.com/PVKonovalov/libiec61850-Go/pkg/iec61850/server"
"github.com/PVKonovalov/libiec61850-Go/pkg/mms"
)

// Create the root IED model
iedModel := imodel.NewIedModel("simpleIOGenericIO")

// Add a logical device
ld := imodel.NewLogicalDevice("simpleIO", iedModel)

// Add logical nodes
lln0 := imodel.NewLogicalNode("LLN0", ld)
ggio1 := imodel.NewLogicalNode("GGIO1", ld)

// Add data objects and attributes
spcso1 := imodel.NewDataObject("SPCSO1", ggio1)
stVal := imodel.NewDataAttribute("stVal", common.FC_ST, common.TypeBoolean, spcso1)
stVal.Value = mms.NewBoolean(false)
stVal.TriggerOptions = common.TriggerDataChanged | common.TriggerQualityChanged

q := imodel.NewDataAttribute("q", common.FC_ST, common.TypeQuality, spcso1)
q.Value = mms.NewBitString([]byte{0x00, 0x00}, 13)

t := imodel.NewDataAttribute("t", common.FC_ST, common.TypeTimestamp, spcso1)
t.Value = mms.NewUTCTime(mms.UTCTimeFromTime(time.Now()))
```

### Starting the Server

```go
iedServer := server.NewIedServer(iedModel, nil) // nil = default config
if err := iedServer.Start(102); err != nil {
log.Fatal(err)
}
defer iedServer.Stop()
```

### Updating Process Values

```go
// Update a value (triggers reporting for subscribed clients)
newVal := mms.NewBoolean(true)
iedServer.UpdateAttributeValue(stVal, newVal)
```

### Write Access Control

```go
iedServer.SetWriteAccessHandler(func (
da *imodel.DataAttribute,
value *mms.Value,
clientAddr net.Addr,
) error {
fmt.Printf("client %s wants to write %s = %s\n", clientAddr, da.Name(), value)
// Return nil to allow, or an error to reject
return nil
})
```

---

## GOOSE Publisher

GOOSE uses raw Ethernet frames (requires root/`CAP_NET_RAW` on Linux).

```go
import (
"github.com/PVKonovalov/libiec61850-Go/pkg/goose"
"github.com/PVKonovalov/libiec61850-Go/pkg/iec61850/common"
"github.com/PVKonovalov/libiec61850-Go/pkg/mms"
)

pub, err := goose.NewPublisher(goose.PublisherConfig{
Interface: "eth0",
CommParams: common.PhyComAddress{
AppID:        1000,
DstAddress:   common.DefaultGooseMulticastAddress(),
VLANPriority: 4,
},
})
if err != nil {
log.Fatal(err)
}
defer pub.Close()

pub.SetGooseCBRef("simpleIO/LLN0$GO$gcbEvents")
pub.SetDataSetRef("simpleIO/LLN0$Events")
pub.SetConfRev(1)
pub.SetTimeAllowedToLive(2000) // ms

// Publish dataset values (increments StNum, resets retransmission counter)
values := []*mms.Value{
mms.NewBoolean(true),
mms.NewFloat32(3.14),
}
pub.Publish(values)
```

---

## GOOSE Subscriber

```go
import "github.com/PVKonovalov/libiec61850-Go/pkg/goose"

// Create a subscriber for a specific App ID
sub := goose.NewSubscriber(1000, func (appID uint16, pdu *goose.GoosePDU) {
fmt.Printf("GOOSE from AppID=%d: StNum=%d SqNum=%d\n",
appID, pdu.StNum, pdu.SqNum)
for i, v := range pdu.AllData {
fmt.Printf("  data[%d] = %s\n", i, v)
}
})
// Optional: filter by control block reference
sub.SetGoCBRef("simpleIO/LLN0$GO$gcbEvents")

// Start the receiver
recv := goose.NewReceiver("eth0")
recv.AddSubscriber(sub)
if err := recv.Start(); err != nil {
log.Fatal(err)
}
defer recv.Stop()
```

---

## MMS Value API

`mms.Value` is the fundamental data type, equivalent to `MmsValue` in the C library.

```go
// Create values
b := mms.NewBoolean(true)
i := mms.NewInt32(42)
u := mms.NewUint32(1000)
f := mms.NewFloat32(3.14)
s := mms.NewVisibleString("hello")
o := mms.NewOctetString([]byte{0x01, 0x02, 0x03})
ts := mms.NewUTCTime(mms.UTCTimeFromTime(time.Now()))

// Array and structure
arr := mms.NewArray([]*mms.Value{
mms.NewInt32(1),
mms.NewInt32(2),
mms.NewInt32(3),
})
str := mms.NewStructure([]*mms.Value{b, f, s})

// Read values
if str.Type() == mms.TypeStructure {
elem0 := str.GetElement(0) // *mms.Value
fmt.Println(elem0.GetBoolean())
}

// Timestamps
ts := mms.UTCTimeFromTime(time.Now())
t := ts.ToTime() // time.Time
```

---

## Functional Constraints

IEC 61850 organizes data attributes into functional constraint groups:

| FC   | Name             | Description                   |
|------|------------------|-------------------------------|
| `ST` | Status           | Binary status, quality        |
| `MX` | Measurands       | Analog measurements           |
| `SP` | Setpoint         | Configurable setpoints        |
| `CF` | Configuration    | Configuration parameters      |
| `DC` | Description      | Textual descriptions          |
| `SG` | Setting group    | Setting group values          |
| `SE` | Setting editable | Editable setting group values |
| `CO` | Control          | Control outputs               |
| `BR` | Buffered report  | BRCB (buffered reporting)     |
| `RP` | Report           | URCB (unbuffered reporting)   |
| `GO` | GOOSE            | GOOSE control blocks          |
| `MS` | Multicast SV     | Multicast sampled values      |

```go
common.FC_ST // Status
common.FC_MX // Measurands
common.FC_CF // Configuration
// ...
```

---

## IEC 61850 Object Reference Format

```
LogicalDevice/LogicalNode.DataObject[.DataAttribute][$FC]

Examples:
  simpleIO/GGIO1.AnIn1.mag.f$MX     — analog measurement float value
  simpleIO/GGIO1.SPCSO1.stVal$ST    — single-point controllable output status
  simpleIO/LLN0.NamPlt.vendor$DC    — description: vendor name
  simpleIO/LLN0.RP.EventsRCB01      — unbuffered report control block
```

---

## Running Examples

```bash
# Build all examples
go build ./examples/...

# Client (requires a running server on localhost:102)
go run ./examples/client_example1/ localhost 102

# Server (TCP port 10102 to avoid requiring root)
go run ./examples/server_simple/ 10102

# GOOSE publisher (requires root or CAP_NET_RAW on Linux)
sudo go run ./examples/goose_publisher/ eth0
```

---

## Testing

```bash
go test ./pkg/...
```

---

## License

This library is licensed under the GNU General Public License v3.0 (GPL-3.0),
matching the license of the original [libiec61850](https://github.com/mz-automation/libiec61850)
C library it is derived from.

---

## Credits

Based on the IEC 61850 standard (IEC 61850-7-2, IEC 61850-8-1, IEC 61850-9-2)
and the reference C implementation [libiec61850](https://github.com/mz-automation/libiec61850)
by Michael Zillgith.
