package common

import (
	"fmt"
	"strings"
)

// FunctionalConstraint represents an IEC 61850 Functional Constraint (FC)
// as defined in IEC 61850-7-2.
type FunctionalConstraint int

const (
	// FC_ST Status information
	FC_ST FunctionalConstraint = 0
	// FC_MX Measurands - analog values
	FC_MX FunctionalConstraint = 1
	// FC_SP Setpoint
	FC_SP FunctionalConstraint = 2
	// FC_SV Substitution
	FC_SV FunctionalConstraint = 3
	// FC_CF Configuration
	FC_CF FunctionalConstraint = 4
	// FC_DC Description
	FC_DC FunctionalConstraint = 5
	// FC_SG Setting group
	FC_SG FunctionalConstraint = 6
	// FC_SE Setting group editable
	FC_SE FunctionalConstraint = 7
	// FC_SR Service response / Service tracking
	FC_SR FunctionalConstraint = 8
	// FC_OR Operate received
	FC_OR FunctionalConstraint = 9
	// FC_BL Blocking
	FC_BL FunctionalConstraint = 10
	// FC_EX Extended definition
	FC_EX FunctionalConstraint = 11
	// FC_CO Control
	FC_CO FunctionalConstraint = 12
	// FC_US Unicast SV
	FC_US FunctionalConstraint = 13
	// FC_MS Multicast SV
	FC_MS FunctionalConstraint = 14
	// FC_RP Unbuffered report
	FC_RP FunctionalConstraint = 15
	// FC_BR Buffered report
	FC_BR FunctionalConstraint = 16
	// FC_LG Log control blocks
	FC_LG FunctionalConstraint = 17
	// FC_GO Goose control blocks
	FC_GO FunctionalConstraint = 18
	// FC_ALL All FCs (wildcard)
	FC_ALL FunctionalConstraint = 99
	// FC_NONE No FC (invalid)
	FC_NONE FunctionalConstraint = -1
)

func (fc FunctionalConstraint) String() string {
	switch fc {
	case FC_ST:
		return "ST"
	case FC_MX:
		return "MX"
	case FC_SP:
		return "SP"
	case FC_SV:
		return "SV"
	case FC_CF:
		return "CF"
	case FC_DC:
		return "DC"
	case FC_SG:
		return "SG"
	case FC_SE:
		return "SE"
	case FC_SR:
		return "SR"
	case FC_OR:
		return "OR"
	case FC_BL:
		return "BL"
	case FC_EX:
		return "EX"
	case FC_CO:
		return "CO"
	case FC_US:
		return "US"
	case FC_MS:
		return "MS"
	case FC_RP:
		return "RP"
	case FC_BR:
		return "BR"
	case FC_LG:
		return "LG"
	case FC_GO:
		return "GO"
	case FC_ALL:
		return "ALL"
	default:
		return fmt.Sprintf("FC(%d)", int(fc))
	}
}

// ParseFC parses a functional constraint string (e.g., "ST", "MX") into a FunctionalConstraint.
func ParseFC(s string) FunctionalConstraint {
	switch strings.ToUpper(s) {
	case "ST":
		return FC_ST
	case "MX":
		return FC_MX
	case "SP":
		return FC_SP
	case "SV":
		return FC_SV
	case "CF":
		return FC_CF
	case "DC":
		return FC_DC
	case "SG":
		return FC_SG
	case "SE":
		return FC_SE
	case "SR":
		return FC_SR
	case "OR":
		return FC_OR
	case "BL":
		return FC_BL
	case "EX":
		return FC_EX
	case "CO":
		return FC_CO
	case "US":
		return FC_US
	case "MS":
		return FC_MS
	case "RP":
		return FC_RP
	case "BR":
		return FC_BR
	case "LG":
		return FC_LG
	case "GO":
		return FC_GO
	case "ALL", "*":
		return FC_ALL
	default:
		return FC_NONE
	}
}
