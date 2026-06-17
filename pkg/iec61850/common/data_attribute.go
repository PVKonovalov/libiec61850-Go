package common

// DataAttributeType defines the primitive data type of an IEC 61850 data attribute.
type DataAttributeType int

const (
	TypeUnknown       DataAttributeType = -1
	TypeBoolean       DataAttributeType = 0
	TypeINT8          DataAttributeType = 1
	TypeINT16         DataAttributeType = 2
	TypeINT32         DataAttributeType = 3
	TypeINT64         DataAttributeType = 4
	TypeINT128        DataAttributeType = 5
	TypeINT8U         DataAttributeType = 6
	TypeINT16U        DataAttributeType = 7
	TypeINT24U        DataAttributeType = 8
	TypeINT32U        DataAttributeType = 9
	TypeFLOAT32       DataAttributeType = 10
	TypeFLOAT64       DataAttributeType = 11
	TypeEnumerated    DataAttributeType = 12
	TypeOctetString64 DataAttributeType = 13
	TypeOctetString6  DataAttributeType = 14
	TypeOctetString8  DataAttributeType = 15
	TypeVisibleStr32  DataAttributeType = 16
	TypeVisibleStr64  DataAttributeType = 17
	TypeVisibleStr65  DataAttributeType = 18
	TypeVisibleStr129 DataAttributeType = 19
	TypeVisibleStr255 DataAttributeType = 20
	TypeUnicodeStr255 DataAttributeType = 21
	TypeTimestamp     DataAttributeType = 22
	TypeQuality       DataAttributeType = 23
	TypeCheck         DataAttributeType = 24
	TypeCodedEnum     DataAttributeType = 25
	TypeGenericBitStr DataAttributeType = 26
	TypeConstructed   DataAttributeType = 27
	TypeEntryTime     DataAttributeType = 28
	TypePhyComAddr    DataAttributeType = 29
	TypeCurrency      DataAttributeType = 30
	TypeOptFlds       DataAttributeType = 31
	TypeTrgOps        DataAttributeType = 32
)
