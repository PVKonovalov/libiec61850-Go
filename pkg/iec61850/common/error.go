package common

import "fmt"

// IedClientError mirrors the IED_ERROR_* codes from the C library.
type IedClientError int

const (
	ErrorOK                                IedClientError = 0
	ErrorNotConnected                      IedClientError = 1
	ErrorAlreadyConnected                  IedClientError = 2
	ErrorConnectionLost                    IedClientError = 3
	ErrorServiceNotSupported               IedClientError = 4
	ErrorConnectionRejected                IedClientError = 5
	ErrorOutstandingCallLimitReached       IedClientError = 6
	ErrorUserProvidedInvalidArgument       IedClientError = 10
	ErrorEnableReportFailedDatasetMismatch IedClientError = 11
	ErrorObjectReferenceInvalid            IedClientError = 12
	ErrorUnexpectedValueReceived           IedClientError = 13
	ErrorTimeout                           IedClientError = 20
	ErrorAccessDenied                      IedClientError = 21
	ErrorObjectDoesNotExist                IedClientError = 22
	ErrorObjectExists                      IedClientError = 23
	ErrorObjectAccessUnsupported           IedClientError = 24
	ErrorTypeInconsistent                  IedClientError = 25
	ErrorTemporarilyUnavailable            IedClientError = 26
	ErrorObjectUndefined                   IedClientError = 27
	ErrorInvalidAddress                    IedClientError = 28
	ErrorHardwareFault                     IedClientError = 29
	ErrorTypeUnsupported                   IedClientError = 30
	ErrorObjectAttributeInconsistent       IedClientError = 31
	ErrorObjectValueInvalid                IedClientError = 32
	ErrorObjectInvalidated                 IedClientError = 33
	ErrorMalformedMessage                  IedClientError = 34
	ErrorObjectConstraintConflict          IedClientError = 35
	ErrorServiceNotImplemented             IedClientError = 98
	ErrorUnknown                           IedClientError = 99
)

func (e IedClientError) Error() string {
	switch e {
	case ErrorOK:
		return "ok"
	case ErrorNotConnected:
		return "not connected"
	case ErrorConnectionLost:
		return "connection lost"
	case ErrorTimeout:
		return "timeout"
	case ErrorAccessDenied:
		return "access denied"
	case ErrorObjectDoesNotExist:
		return "object does not exist"
	case ErrorObjectExists:
		return "object already exists"
	case ErrorConnectionRejected:
		return "connection rejected"
	default:
		return fmt.Sprintf("IedClientError(%d)", int(e))
	}
}
