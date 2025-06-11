package common

import (
	"github.com/zclconf/go-cty/cty"
)

// DiagnosticSeverity represents the severity level of a diagnostic
type DiagnosticSeverity int

const (
	Error DiagnosticSeverity = iota
	Warning
)

// Diagnostic represents a single diagnostic message
type Diagnostic struct {
	Severity  DiagnosticSeverity
	Summary   string
	Detail    string
	Attribute cty.Path
}

// Diagnostics represents a collection of diagnostic messages
type Diagnostics []Diagnostic

// HasErrors returns true if any diagnostic has Error severity
func (diags Diagnostics) HasErrors() bool {
	for _, diag := range diags {
		if diag.Severity == Error {
			return true
		}
	}
	return false
}

// ErrorDiagnostics creates a diagnostic with Error severity from an error
func ErrorDiagnostics(summary, detail string, err error) Diagnostics {
	return Diagnostics{
		{
			Severity: Error,
			Summary:  summary,
			Detail:   detail + ": " + err.Error(),
		},
	}
}

// RPCErrorDiagnostics creates a diagnostic for RPC errors
func RPCErrorDiagnostics(err error) Diagnostics {
	if err == nil {
		return nil
	}
	return Diagnostics{
		{
			Severity: Error,
			Summary:  "RPC communication error",
			Detail:   "Error while calling provider: " + err.Error(),
		},
	}
}