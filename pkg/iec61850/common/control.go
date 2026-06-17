package common

// ControlModel defines how a controllable data object is operated.
type ControlModel int

const (
	// ControlModelStatusOnly no control, status information only
	ControlModelStatusOnly ControlModel = 0
	// ControlModelDirectNormal direct control with normal security
	ControlModelDirectNormal ControlModel = 1
	// ControlModelSBONormal select-before-operate with normal security
	ControlModelSBONormal ControlModel = 2
	// ControlModelDirectEnhanced direct control with enhanced security
	ControlModelDirectEnhanced ControlModel = 3
	// ControlModelSBOEnhanced select-before-operate with enhanced security
	ControlModelSBOEnhanced ControlModel = 4
)

// OriginatorCategory defines who initiated a control operation.
type OriginatorCategory int

const (
	OrCatNotSupported     OriginatorCategory = 0
	OrCatBayControl       OriginatorCategory = 1
	OrCatStationControl   OriginatorCategory = 2
	OrCatRemoteControl    OriginatorCategory = 3
	OrCatAutomaticBay     OriginatorCategory = 4
	OrCatAutomaticStation OriginatorCategory = 5
	OrCatAutomaticRemote  OriginatorCategory = 6
	OrCatMaintenance      OriginatorCategory = 7
	OrCatProcess          OriginatorCategory = 8
)

// ControlAddCause holds the additional cause for control model errors.
type ControlAddCause int

const (
	AddCauseUnknown                     ControlAddCause = 0
	AddCauseNotSupported                ControlAddCause = 1
	AddCauseBlockedBySwitchingHierarchy ControlAddCause = 2
	AddCauseSelectFailed                ControlAddCause = 3
	AddCauseInvalidPosition             ControlAddCause = 4
	AddCausePositionReached             ControlAddCause = 5
	AddCauseParameterChangeInExecution  ControlAddCause = 6
	AddCauseStepLimit                   ControlAddCause = 7
	AddCauseBlockedByMode               ControlAddCause = 8
	AddCauseBlockedByProcess            ControlAddCause = 9
	AddCauseBlockedByInterlocking       ControlAddCause = 10
	AddCauseBlockedBySynchrocheck       ControlAddCause = 11
	AddCauseCommandAlreadyInExecution   ControlAddCause = 12
	AddCauseBlockedByHealth             ControlAddCause = 13
	AddCause1OfNControl                 ControlAddCause = 14
	AddCauseAbortionByCancel            ControlAddCause = 15
	AddCauseTimeLimitOver               ControlAddCause = 16
	AddCauseAbortionByTrip              ControlAddCause = 17
	AddCauseObjectNotSelected           ControlAddCause = 18
	AddCauseObjectAlreadySelected       ControlAddCause = 19
	AddCauseNoAccessAuthority           ControlAddCause = 20
	AddCauseEndedWithOvershoot          ControlAddCause = 21
	AddCauseAbortionDueToDeviation      ControlAddCause = 22
	AddCauseAbortionByCommunicationLoss ControlAddCause = 23
	AddCauseAbortionByCommand           ControlAddCause = 24
	AddCauseNone                        ControlAddCause = 25
	AddCauseInconsistentParameters      ControlAddCause = 26
	AddCauseLockedByOtherClient         ControlAddCause = 27
)
