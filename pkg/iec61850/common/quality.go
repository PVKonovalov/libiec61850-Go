package common

// Quality represents the IEC 61850 quality bitstring (q) attribute.
// It encodes validity, source, and supplemental information.
type Quality uint16

const (
	// Quality validity bits (bits 0-1)
	QualityGood         Quality = 0x0000
	QualityInvalid      Quality = 0x0001
	QualityReserved     Quality = 0x0002
	QualityQuestionable Quality = 0x0003
	QualityValidityMask Quality = 0x0003

	// Quality detail flags
	QualityOverflow        Quality = 0x0004
	QualityOutOfRange      Quality = 0x0008
	QualityBadReference    Quality = 0x0010
	QualityOscillatory     Quality = 0x0020
	QualityFailure         Quality = 0x0040
	QualityOldData         Quality = 0x0080
	QualityInconsistent    Quality = 0x0100
	QualityInaccurate      Quality = 0x0200
	QualitySource          Quality = 0x0400 // 0=process, 1=substituted
	QualityTest            Quality = 0x0800
	QualityOperatorBlocked Quality = 0x1000
)

// Validity returns the validity portion of the quality.
func (q Quality) Validity() Quality { return q & QualityValidityMask }

// IsGood returns true if the quality is GOOD (valid, no issues).
func (q Quality) IsGood() bool { return q == QualityGood }

// IsInvalid returns true if the validity bits indicate INVALID.
func (q Quality) IsInvalid() bool { return q.Validity() == QualityInvalid }

// IsQuestionable returns true if the validity bits indicate QUESTIONABLE.
func (q Quality) IsQuestionable() bool { return q.Validity() == QualityQuestionable }

// IsSubstituted returns true if the value is substituted (not from process).
func (q Quality) IsSubstituted() bool { return q&QualitySource != 0 }

// IsTest returns true if the test flag is set.
func (q Quality) IsTest() bool { return q&QualityTest != 0 }

// IsOperatorBlocked returns true if the operator-blocked flag is set.
func (q Quality) IsOperatorBlocked() bool { return q&QualityOperatorBlocked != 0 }
