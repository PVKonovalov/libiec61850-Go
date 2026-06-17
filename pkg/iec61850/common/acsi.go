package common

// ACSIClass represents an IEC 61850 ACSI service class.
type ACSIClass int

const (
	ACSIClassDataObject ACSIClass = iota
	ACSIClassDataSet
	ACSIClassBRCB // Buffered Report Control Block
	ACSIClassURCB // Unbuffered Report Control Block
	ACSIClassLCB  // Log Control Block
	ACSIClassLog
	ACSIClassSGCB  // Setting Group Control Block
	ACSIClassGoCB  // GOOSE Control Block
	ACSIClassGsCB  // GSSE Control Block
	ACSIClassMSVCB // Multicast Sampled Value CB
	ACSIClassUSVCB // Unicast Sampled Value CB
)
