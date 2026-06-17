package common

import (
	"fmt"
	"strings"
)

// ObjectReference represents a full IEC 61850 object reference.
// Format: LDName/LNName.DOName[.DAName][.SubDA...][$FC]
type ObjectReference struct {
	LogicalDevice string
	LogicalNode   string
	DataObject    string
	DataAttribute string
	FC            FunctionalConstraint
}

// ParseObjectReference parses an IEC 61850 object reference string.
// Accepted format: "LDName/LNClass.DOName[.DAName][$FC]"
func ParseObjectReference(ref string) (ObjectReference, error) {
	or := ObjectReference{FC: FC_NONE}

	// Split on '$' to extract FC
	parts := strings.SplitN(ref, "$", 2)
	if len(parts) == 2 {
		or.FC = ParseFC(parts[1])
		ref = parts[0]
	}

	// Split on '/' to separate LD from LN.DO.DA
	slashIdx := strings.IndexByte(ref, '/')
	if slashIdx < 0 {
		return or, fmt.Errorf("iec61850: missing '/' in object reference %q", ref)
	}
	or.LogicalDevice = ref[:slashIdx]
	rest := ref[slashIdx+1:]

	// Split on '.' to get LN, DO, DA
	dotParts := strings.SplitN(rest, ".", 3)
	if len(dotParts) < 2 {
		return or, fmt.Errorf("iec61850: expected LN.DO in %q", rest)
	}
	or.LogicalNode = dotParts[0]
	or.DataObject = dotParts[1]
	if len(dotParts) == 3 {
		or.DataAttribute = dotParts[2]
	}
	return or, nil
}

// String returns the canonical IEC 61850 object reference string.
func (or ObjectReference) String() string {
	s := or.LogicalDevice + "/" + or.LogicalNode + "." + or.DataObject
	if or.DataAttribute != "" {
		s += "." + or.DataAttribute
	}
	if or.FC != FC_NONE && or.FC != FC_ALL {
		s += "$" + or.FC.String()
	}
	return s
}

// MMSObjectReference converts an IEC 61850 object reference to the MMS
// domain-specific variable name used on the wire.
// Format: domainId = LDName, itemId = LNName$FC$DOName[$DAName]
func (or ObjectReference) MMSObjectReference() (domainID, itemID string) {
	domainID = or.LogicalDevice
	if or.FC == FC_NONE || or.FC == FC_ALL {
		itemID = or.LogicalNode + "." + or.DataObject
		if or.DataAttribute != "" {
			itemID += "." + or.DataAttribute
		}
	} else {
		itemID = or.LogicalNode + "$" + or.FC.String() + "$" + or.DataObject
		if or.DataAttribute != "" {
			itemID += "$" + or.DataAttribute
		}
	}
	return
}
