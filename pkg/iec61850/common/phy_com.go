package common

// PhyComAddress holds Ethernet/VLAN communication parameters for GOOSE/SV.
type PhyComAddress struct {
	VLANPriority uint8
	VLANID       uint16
	AppID        uint16
	DstAddress   [6]byte
}

// DefaultGooseMulticastAddress returns the default GOOSE multicast MAC address.
func DefaultGooseMulticastAddress() [6]byte {
	return [6]byte{0x01, 0x0C, 0xCD, 0x01, 0x00, 0x00}
}

// DefaultSVMulticastAddress returns the default Sampled Values multicast MAC address.
func DefaultSVMulticastAddress() [6]byte {
	return [6]byte{0x01, 0x0C, 0xCD, 0x04, 0x00, 0x00}
}
