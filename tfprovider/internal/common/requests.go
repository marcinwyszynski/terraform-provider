package common

import (
	"github.com/zclconf/go-cty/cty"
)

type ManagedResourceReadRequest struct {
	PreviousValue cty.Value
	OpaquePrivate []byte
}

type ManagedResourceReadResponse struct {
	RefreshedValue cty.Value
	OpaquePrivate  []byte
}

// PlanResourceChangeRequest represents a request to plan a resource change.
type PlanResourceChangeRequest struct {
	TypeName         string
	PriorState       cty.Value
	ProposedNewState cty.Value
	Config           cty.Value
	PriorPrivate     []byte
	ProviderMeta     cty.Value
}

// PlanResourceChangeResponse represents the response from planning a resource change.
type PlanResourceChangeResponse struct {
	PlannedState    cty.Value
	RequiresReplace []cty.Path
	PlannedPrivate  []byte
}

// ApplyResourceChangeRequest represents a request to apply a resource change.
type ApplyResourceChangeRequest struct {
	TypeName       string
	PriorState     cty.Value
	PlannedState   cty.Value
	Config         cty.Value
	PlannedPrivate []byte
	ProviderMeta   cty.Value
}

// ApplyResourceChangeResponse represents the response from applying a resource change.
type ApplyResourceChangeResponse struct {
	NewState cty.Value
	Private  []byte
}

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

// ImportResourceStateRequest represents a request to import a resource.
type ImportResourceStateRequest struct {
	TypeName string
	ID       string
}

// ImportedResource represents a single imported resource in the response.
type ImportedResource struct {
	TypeName string
	State    cty.Value
	Private  []byte
}

// ImportResourceStateResponse represents the response from importing a resource.
type ImportResourceStateResponse struct {
	ImportedResources []ImportedResource
}
