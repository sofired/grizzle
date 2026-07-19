package query

import "github.com/sofired/grizzle/expr"

// ErrorCode is the stable programmatic classification for query build and
// execution failures.
type ErrorCode = expr.ErrorCode

// Error is the shared redacted Grizzle error shape.
type Error = expr.Error

const (
	CodeUnsupportedFeature  = expr.CodeUnsupportedFeature
	CodeUnsupportedDialect  = expr.CodeUnsupportedDialect
	CodeInvalidIdentifier   = expr.CodeInvalidIdentifier
	CodePreparedNotReady    = expr.CodePreparedNotReady
	CodeRegistryClosed      = expr.CodeRegistryClosed
	CodeMissingParam        = expr.CodeMissingParam
	CodeInvalidParamType    = expr.CodeInvalidParamType
	CodeInvalidParamValue   = expr.CodeInvalidParamValue
	CodeParamEncode         = expr.CodeParamEncode
	CodeInvalidResultKind   = expr.CodeInvalidResultKind
	CodeDuplicateRegistry   = expr.CodeDuplicateRegistry
	CodePreparedTxMismatch  = expr.CodePreparedTxMismatch
	CodeInvalidReceiver     = expr.CodeInvalidReceiver
	CodeBuildValidation     = expr.CodeBuildValidation
	CodeNotFound            = expr.CodeNotFound
	CodeTooManyRows         = expr.CodeTooManyRows
	CodeInvalidRows         = expr.CodeInvalidRows
	CodeScanDecode          = expr.CodeScanDecode
	CodeTransactionBegin    = expr.CodeTransactionBegin
	CodeTransactionCommit   = expr.CodeTransactionCommit
	CodeTransactionRollback = expr.CodeTransactionRollback
	CodeTransactionCallback = expr.CodeTransactionCallback
)

var (
	ErrUnsupportedFeature  = expr.ErrUnsupportedFeature
	ErrUnsupportedDialect  = expr.ErrUnsupportedDialect
	ErrInvalidIdentifier   = expr.ErrInvalidIdentifier
	ErrPreparedNotReady    = expr.ErrPreparedNotReady
	ErrRegistryClosed      = expr.ErrRegistryClosed
	ErrMissingParam        = expr.ErrMissingParam
	ErrInvalidParamType    = expr.ErrInvalidParamType
	ErrInvalidParamValue   = expr.ErrInvalidParamValue
	ErrParamEncode         = expr.ErrParamEncode
	ErrInvalidResultKind   = expr.ErrInvalidResultKind
	ErrDuplicateRegistry   = expr.ErrDuplicateRegistry
	ErrPreparedTxMismatch  = expr.ErrPreparedTxMismatch
	ErrInvalidReceiver     = expr.ErrInvalidReceiver
	ErrBuildValidation     = expr.ErrBuildValidation
	ErrNotFound            = expr.ErrNotFound
	ErrTooManyRows         = expr.ErrTooManyRows
	ErrInvalidRows         = expr.ErrInvalidRows
	ErrScanDecode          = expr.ErrScanDecode
	ErrTransactionBegin    = expr.ErrTransactionBegin
	ErrTransactionCommit   = expr.ErrTransactionCommit
	ErrTransactionRollback = expr.ErrTransactionRollback
	ErrTransactionCallback = expr.ErrTransactionCallback
)

// NewError returns a stable, redacted query error.
func NewError(code ErrorCode, op, message string) *Error {
	return expr.NewError(code, op, message)
}
