package expr

import (
	"context"
	"errors"
	"fmt"
	"io"
)

// ErrorCode is a stable programmatic classification for build and execution
// failures. Callers should prefer errors.Is and errors.As over matching error
// strings.
type ErrorCode string

const (
	CodeUnsupportedFeature  ErrorCode = "unsupported_feature"
	CodeUnsupportedDialect  ErrorCode = "unsupported_dialect"
	CodeInvalidIdentifier   ErrorCode = "invalid_identifier"
	CodePreparedNotReady    ErrorCode = "prepared_not_ready"
	CodeRegistryClosed      ErrorCode = "registry_closed"
	CodeMissingParam        ErrorCode = "missing_param"
	CodeInvalidParamType    ErrorCode = "invalid_param_type"
	CodeInvalidParamValue   ErrorCode = "invalid_param_value"
	CodeParamEncode         ErrorCode = "param_encode"
	CodeInvalidResultKind   ErrorCode = "invalid_prepared_result_kind"
	CodeDuplicateRegistry   ErrorCode = "duplicate_registry_name"
	CodePreparedTxMismatch  ErrorCode = "prepared_tx_mismatch"
	CodeInvalidReceiver     ErrorCode = "invalid_receiver"
	CodeBuildValidation     ErrorCode = "build_validation"
	CodeNotFound            ErrorCode = "not_found"
	CodeTooManyRows         ErrorCode = "too_many_rows"
	CodeInvalidRows         ErrorCode = "invalid_rows"
	CodeScanDecode          ErrorCode = "scan_decode"
	CodeTransactionBegin    ErrorCode = "transaction_begin"
	CodeTransactionCommit   ErrorCode = "transaction_commit"
	CodeTransactionRollback ErrorCode = "transaction_rollback"
	CodeTransactionCallback ErrorCode = "transaction_callback"
)

var (
	ErrUnsupportedFeature  = errors.New(string(CodeUnsupportedFeature))
	ErrUnsupportedDialect  = errors.New(string(CodeUnsupportedDialect))
	ErrInvalidIdentifier   = errors.New(string(CodeInvalidIdentifier))
	ErrPreparedNotReady    = errors.New(string(CodePreparedNotReady))
	ErrRegistryClosed      = errors.New(string(CodeRegistryClosed))
	ErrMissingParam        = errors.New(string(CodeMissingParam))
	ErrInvalidParamType    = errors.New(string(CodeInvalidParamType))
	ErrInvalidParamValue   = errors.New(string(CodeInvalidParamValue))
	ErrParamEncode         = errors.New(string(CodeParamEncode))
	ErrInvalidResultKind   = errors.New(string(CodeInvalidResultKind))
	ErrDuplicateRegistry   = errors.New(string(CodeDuplicateRegistry))
	ErrPreparedTxMismatch  = errors.New(string(CodePreparedTxMismatch))
	ErrInvalidReceiver     = errors.New(string(CodeInvalidReceiver))
	ErrBuildValidation     = errors.New(string(CodeBuildValidation))
	ErrNotFound            = errors.New(string(CodeNotFound))
	ErrTooManyRows         = errors.New(string(CodeTooManyRows))
	ErrInvalidRows         = errors.New(string(CodeInvalidRows))
	ErrScanDecode          = errors.New(string(CodeScanDecode))
	ErrTransactionBegin    = errors.New(string(CodeTransactionBegin))
	ErrTransactionCommit   = errors.New(string(CodeTransactionCommit))
	ErrTransactionRollback = errors.New(string(CodeTransactionRollback))
	ErrTransactionCallback = errors.New(string(CodeTransactionCallback))
)

// Error is a redacted Grizzle error with a stable code and operation.
// Err, when set by Grizzle, contains only a safe sentinel such as context
// cancellation; raw SQL, values, identifiers, and driver errors are never
// stored here.
type Error struct {
	Code    ErrorCode
	Op      string
	Message string
	Err     error
}

// NewError returns a stable, redacted build error.
func NewError(code ErrorCode, op, message string) *Error {
	return &Error{Code: code, Op: op, Message: message}
}

// Error returns a redacted diagnostic containing no SQL or input values.
func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	message := e.Message
	if message == "" {
		message = string(e.Code)
	}
	if e.Op == "" {
		return message
	}
	return e.Op + ": " + message
}

// Unwrap exposes only stable sentinels and the standard context sentinels.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	if errors.Is(e.Err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(e.Err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return sentinelForCode(e.Code)
}

// Is preserves both the stable Grizzle classification and safe context
// cancellation sentinels when Err carries a context cause.
func (e *Error) Is(target error) bool {
	if e == nil {
		return false
	}
	if sentinel := sentinelForCode(e.Code); sentinel != nil && target == sentinel {
		return true
	}
	switch target {
	case context.Canceled:
		return errors.Is(e.Err, context.Canceled)
	case context.DeadlineExceeded:
		return errors.Is(e.Err, context.DeadlineExceeded)
	default:
		return false
	}
}

// Format prevents %+v from exposing the Error struct or its Err field.
func (e *Error) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v', 's', 'q':
		if verb == 'q' {
			_, _ = fmt.Fprintf(s, "%q", e.Error())
			return
		}
		_, _ = io.WriteString(s, e.Error())
	default:
		_, _ = io.WriteString(s, e.Error())
	}
}

func sentinelForCode(code ErrorCode) error {
	switch code {
	case CodeUnsupportedFeature:
		return ErrUnsupportedFeature
	case CodeUnsupportedDialect:
		return ErrUnsupportedDialect
	case CodeInvalidIdentifier:
		return ErrInvalidIdentifier
	case CodePreparedNotReady:
		return ErrPreparedNotReady
	case CodeRegistryClosed:
		return ErrRegistryClosed
	case CodeMissingParam:
		return ErrMissingParam
	case CodeInvalidParamType:
		return ErrInvalidParamType
	case CodeInvalidParamValue:
		return ErrInvalidParamValue
	case CodeParamEncode:
		return ErrParamEncode
	case CodeInvalidResultKind:
		return ErrInvalidResultKind
	case CodeDuplicateRegistry:
		return ErrDuplicateRegistry
	case CodePreparedTxMismatch:
		return ErrPreparedTxMismatch
	case CodeInvalidReceiver:
		return ErrInvalidReceiver
	case CodeBuildValidation:
		return ErrBuildValidation
	case CodeNotFound:
		return ErrNotFound
	case CodeTooManyRows:
		return ErrTooManyRows
	case CodeInvalidRows:
		return ErrInvalidRows
	case CodeScanDecode:
		return ErrScanDecode
	case CodeTransactionBegin:
		return ErrTransactionBegin
	case CodeTransactionCommit:
		return ErrTransactionCommit
	case CodeTransactionRollback:
		return ErrTransactionRollback
	case CodeTransactionCallback:
		return ErrTransactionCallback
	default:
		return nil
	}
}
