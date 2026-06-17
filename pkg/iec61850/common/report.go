package common

// TriggerOption defines when a report is triggered. Multiple options can be OR'd.
type TriggerOption uint8

const (
	TriggerDataChanged    TriggerOption = 0x01
	TriggerQualityChanged TriggerOption = 0x02
	TriggerDataUpdate     TriggerOption = 0x04
	TriggerIntegrity      TriggerOption = 0x08
	TriggerGI             TriggerOption = 0x10
	TriggerTransient      TriggerOption = 0x80
)

// ReportOption defines what is included in a report. Multiple options can be OR'd.
type ReportOption uint8

const (
	ReportOptSeqNum             ReportOption = 0x01
	ReportOptTimeStamp          ReportOption = 0x02
	ReportOptReasonForInclusion ReportOption = 0x04
	ReportOptDataSet            ReportOption = 0x08
	ReportOptDataReference      ReportOption = 0x10
	ReportOptBufferOverflow     ReportOption = 0x20
	ReportOptEntryID            ReportOption = 0x40
	ReportOptConfRev            ReportOption = 0x80
)

// ReasonForInclusion indicates why a data set member was included in a report.
type ReasonForInclusion int

const (
	ReasonNotIncluded        ReasonForInclusion = 0
	ReasonDataChange         ReasonForInclusion = 1
	ReasonQualityChange      ReasonForInclusion = 2
	ReasonDataUpdate         ReasonForInclusion = 3
	ReasonIntegrity          ReasonForInclusion = 4
	ReasonGI                 ReasonForInclusion = 5
	ReasonApplicationTrigger ReasonForInclusion = 6
)
