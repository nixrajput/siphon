// Package errs defines siphon's typed error model and exit-code taxonomy.
package errs

import (
	"errors"
)

// Sentinel errors. Use errors.Is / errors.As to match.
var (
	ErrProfileNotFound    = errors.New("profile not found")
	ErrProfileInvalid     = errors.New("profile schema invalid")
	ErrSecretUnresolved   = errors.New("secret reference could not be resolved")
	ErrDriverUnsupported  = errors.New("driver not supported")
	ErrToolMissing        = errors.New("required external tool not found")
	ErrConnectionFailed   = errors.New("database connection failed")
	ErrChecksumMismatch   = errors.New("dump checksum mismatch")
	ErrDumpCorrupt        = errors.New("dump file corrupt or incomplete")
	ErrIncompatibleEngine = errors.New("source/target engine incompatible")
	ErrMissingPrimaryKey  = errors.New("change has no primary key to target")
	Err2FARequired        = errors.New("two-factor authentication required")
	ErrCancelled          = errors.New("cancelled by user")
)

// Code is the exit-code taxonomy bucket. Matches the spec table in §4.2.
type Code int

const (
	CodeOK         Code = 0
	CodeUser       Code = 1
	CodeSystem     Code = 2
	CodeIntegrity  Code = 3
	CodePartial    Code = 4
	CodeCancelled  Code = 130
	CodeTerminated Code = 143
)

// ExitCode returns the POSIX exit code for this Code.
func (c Code) ExitCode() int { return int(c) }

// Error is the structured error type all Application verbs return.
type Error struct {
	Op    string // the verb name, e.g. "backup", "restore"
	Code  Code   // exit-code taxonomy bucket
	Cause error  // underlying cause; matched by errors.Is / errors.As
	Hint  string // user-actionable remediation, rendered in CLI/TUI display
}

func (e *Error) Error() string {
	msg := e.Op + " failed"
	if e.Cause != nil {
		msg += ": " + e.Cause.Error()
	}
	if e.Hint != "" {
		msg += " (" + e.Hint + ")"
	}
	return msg
}

func (e *Error) Unwrap() error { return e.Cause }
