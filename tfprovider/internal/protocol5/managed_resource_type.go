package protocol5

import (
	"context"
	"fmt"

	"github.com/apparentlymart/terraform-schema-go/tfschema"
	"github.com/zclconf/go-cty/cty"

	"github.com/apparentlymart/terraform-provider/internal/tfplugin5"
	"github.com/apparentlymart/terraform-provider/tfprovider/internal/common"
)

type ManagedResourceType struct {
	client             tfplugin5.ProviderClient
	typeName           string
	schema             *common.ManagedResourceTypeSchema
	providerMetaSchema *tfschema.Block
}

func (rt *ManagedResourceType) ValidateConfig(ctx context.Context, config cty.Value) common.Diagnostics {
	dv, diags := encodeDynamicValue(config, rt.schema.Content)
	if diags.HasErrors() {
		return diags
	}
	resp, err := rt.client.ValidateResourceTypeConfig(ctx, &tfplugin5.ValidateResourceTypeConfig_Request{
		TypeName: rt.typeName,
		Config:   dv,
	})
	diags = append(diags, common.RPCErrorDiagnostics(err)...)
	if err != nil {
		return diags
	}
	diags = append(diags, decodeDiagnostics(resp.Diagnostics)...)
	return diags
}

func (rt *ManagedResourceType) Read(ctx context.Context, req common.ManagedResourceReadRequest) (common.ManagedResourceReadResponse, common.Diagnostics) {
	resp := common.ManagedResourceReadResponse{}
	dv, diags := encodeDynamicValue(req.PreviousValue, rt.schema.Content)
	if diags.HasErrors() {
		return resp, diags
	}

	rawResp, err := rt.client.ReadResource(ctx, &tfplugin5.ReadResource_Request{
		TypeName:     rt.typeName,
		CurrentState: dv,
		Private:      req.OpaquePrivate,
	})
	diags = append(diags, common.RPCErrorDiagnostics(err)...)
	if err != nil {
		return resp, diags
	}
	diags = append(diags, decodeDiagnostics(rawResp.Diagnostics)...)

	if raw := rawResp.NewState; raw != nil {
		v, moreDiags := decodeDynamicValue(raw, rt.schema.Content)
		resp.RefreshedValue = v
		diags = append(diags, moreDiags...)
	}
	resp.OpaquePrivate = rawResp.Private
	return resp, diags
}

func (rt *ManagedResourceType) Plan(ctx context.Context, req common.ManagedResourcePlanRequest) (common.ManagedResourcePlanResponse, common.Diagnostics) {
	var diags common.Diagnostics

	priorDV, moreDiags := encodeDynamicValue(req.PriorState, rt.schema.Content)
	diags = append(diags, moreDiags...)
	if moreDiags.HasErrors() {
		return common.ManagedResourcePlanResponse{}, diags
	}

	proposedDV, moreDiags := encodeDynamicValue(req.ProposedNewState, rt.schema.Content)
	diags = append(diags, moreDiags...)
	if moreDiags.HasErrors() {
		return common.ManagedResourcePlanResponse{}, diags
	}

	configDV, moreDiags := encodeDynamicValue(req.Config, rt.schema.Content)
	diags = append(diags, moreDiags...)
	if moreDiags.HasErrors() {
		return common.ManagedResourcePlanResponse{}, diags
	}

	var providerMetaDV *tfplugin5.DynamicValue
	if !req.ProviderMeta.IsNull() && rt.providerMetaSchema != nil {
		var moreDiags common.Diagnostics
		providerMetaDV, moreDiags = encodeDynamicValue(req.ProviderMeta, rt.providerMetaSchema)
		diags = append(diags, moreDiags...)
		if moreDiags.HasErrors() {
			return common.ManagedResourcePlanResponse{}, diags
		}
	}

	resp, err := rt.client.PlanResourceChange(ctx, &tfplugin5.PlanResourceChange_Request{
		TypeName:         rt.typeName,
		PriorState:       priorDV,
		ProposedNewState: proposedDV,
		Config:           configDV,
		PriorPrivate:     req.OpaquePrivate,
		ProviderMeta:     providerMetaDV,
	})
	diags = append(diags, common.RPCErrorDiagnostics(err)...)
	if err != nil {
		return common.ManagedResourcePlanResponse{}, diags
	}

	diags = append(diags, decodeDiagnostics(resp.Diagnostics)...)

	result := common.ManagedResourcePlanResponse{
		OpaquePrivate: resp.PlannedPrivate,
	}

	if resp.PlannedState != nil {
		plannedState, moreDiags := decodeDynamicValue(resp.PlannedState, rt.schema.Content)
		diags = append(diags, moreDiags...)
		result.PlannedState = plannedState
	}

	for _, attrPath := range resp.RequiresReplace {
		path := decodeAttributePath(attrPath)
		result.RequiresReplace = append(result.RequiresReplace, path)
	}

	return result, diags
}

func (rt *ManagedResourceType) Apply(ctx context.Context, req common.ManagedResourceApplyRequest) (common.ManagedResourceApplyResponse, common.Diagnostics) {
	var diags common.Diagnostics

	priorDV, moreDiags := encodeDynamicValue(req.PriorState, rt.schema.Content)
	diags = append(diags, moreDiags...)
	if moreDiags.HasErrors() {
		return common.ManagedResourceApplyResponse{}, diags
	}

	plannedDV, moreDiags := encodeDynamicValue(req.PlannedState, rt.schema.Content)
	diags = append(diags, moreDiags...)
	if moreDiags.HasErrors() {
		return common.ManagedResourceApplyResponse{}, diags
	}

	configDV, moreDiags := encodeDynamicValue(req.Config, rt.schema.Content)
	diags = append(diags, moreDiags...)
	if moreDiags.HasErrors() {
		return common.ManagedResourceApplyResponse{}, diags
	}

	var providerMetaDV *tfplugin5.DynamicValue
	if !req.ProviderMeta.IsNull() && rt.providerMetaSchema != nil {
		var moreDiags common.Diagnostics
		providerMetaDV, moreDiags = encodeDynamicValue(req.ProviderMeta, rt.providerMetaSchema)
		diags = append(diags, moreDiags...)
		if moreDiags.HasErrors() {
			return common.ManagedResourceApplyResponse{}, diags
		}
	}

	resp, err := rt.client.ApplyResourceChange(ctx, &tfplugin5.ApplyResourceChange_Request{
		TypeName:       rt.typeName,
		PriorState:     priorDV,
		PlannedState:   plannedDV,
		Config:         configDV,
		PlannedPrivate: req.OpaquePrivate,
		ProviderMeta:   providerMetaDV,
	})
	diags = append(diags, common.RPCErrorDiagnostics(err)...)
	if err != nil {
		return common.ManagedResourceApplyResponse{}, diags
	}

	diags = append(diags, decodeDiagnostics(resp.Diagnostics)...)

	result := common.ManagedResourceApplyResponse{
		OpaquePrivate: resp.Private,
	}

	if resp.NewState != nil {
		newState, moreDiags := decodeDynamicValue(resp.NewState, rt.schema.Content)
		diags = append(diags, moreDiags...)
		result.NewState = newState
	}

	return result, diags
}

func (rt *ManagedResourceType) Import(ctx context.Context, req common.ManagedResourceImportRequest) (common.ManagedResourceImportResponse, common.Diagnostics) {
	var diags common.Diagnostics

	resp, err := rt.client.ImportResourceState(ctx, &tfplugin5.ImportResourceState_Request{
		TypeName: rt.typeName,
		Id:       req.ID,
	})
	diags = append(diags, common.RPCErrorDiagnostics(err)...)
	if err != nil {
		return common.ManagedResourceImportResponse{}, diags
	}

	diags = append(diags, decodeDiagnostics(resp.Diagnostics)...)

	result := common.ManagedResourceImportResponse{}

	for _, imported := range resp.ImportedResources {
		// Validate that imported resource type matches expected type
		if imported.TypeName != rt.typeName {
			diags = append(diags, common.Diagnostic{
				Severity: common.Error,
				Summary:  "Import type mismatch",
				Detail:   fmt.Sprintf("Expected resource type %q, but provider returned %q during import", rt.typeName, imported.TypeName),
			})
			continue
		}
		state, moreDiags := decodeDynamicValue(imported.State, rt.schema.Content)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			result.ImportedResources = append(result.ImportedResources, common.ImportedResource{
				TypeName:      imported.TypeName,
				State:         state,
				OpaquePrivate: imported.Private,
			})
		}
	}

	return result, diags
}

func (rt *ManagedResourceType) Sealed() common.Sealed {
	return common.Sealed{}
}
