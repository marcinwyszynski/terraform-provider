package common

import (
	"github.com/zclconf/go-cty/cty"
)

// ReadResourceRequest represents a request to read a resource's current state.
type ReadResourceRequest struct {
	TypeName     string
	CurrentState cty.Value
	Private      []byte
	ProviderMeta cty.Value
}

// ReadResourceResponse represents the response from reading a resource.
type ReadResourceResponse struct {
	NewState cty.Value
	Private  []byte
}