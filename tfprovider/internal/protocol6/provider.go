package protocol6

import (
	"context"
	"fmt"
	"sync"

	"github.com/apparentlymart/terraform-provider/internal/tfplugin6"
	"github.com/apparentlymart/terraform-provider/tfprovider/internal/common"
	"github.com/zclconf/go-cty/cty"
	"go.rpcplugin.org/rpcplugin"
)

// Provider is the implementation of tfprovider.Provider for provider plugin
// protocol version 5.
type Provider struct {
	client tfplugin6.ProviderClient
	plugin *rpcplugin.Plugin
	schema *common.Schema

	configured   bool
	configuredMu *sync.Mutex
}

func NewProvider(ctx context.Context, plugin *rpcplugin.Plugin, clientProxy interface{}) (*Provider, error) {
	client := clientProxy.(tfplugin6.ProviderClient)

	// We proactively fetch the schema here because you can't really do anything
	// useful to a provider without it: we need it to serialize any values given
	// in msgpack format.
	schema, err := loadSchema(ctx, client)
	if err != nil {
		return nil, err
	}

	return &Provider{
		client:     client,
		plugin:     plugin,
		schema:     schema,
		configured: false,
	}, nil
}

func (p *Provider) Sealed() common.Sealed {
	return common.Sealed{}
}

func (p *Provider) Schema(ctx context.Context) (*common.Schema, common.Diagnostics) {
	return p.schema, nil
}

func (p *Provider) PrepareConfig(ctx context.Context, config cty.Value) (common.Config, common.Diagnostics) {
	// We're encoding the value here only for the side-effect of making sure
	// it _can_ be encoded using the schema, because in tfplugin5 this is where
	// we would've asked the provider to pre-validate the config but tfplugin6
	// doesn't have that separate step anymore.
	_, diags := encodeDynamicValue(config, p.schema.ProviderConfig)
	if diags.HasErrors() {
		return common.Config{config}, diags
	}
	return common.Config{config}, diags
}

func (p *Provider) Configure(ctx context.Context, config common.Config) common.Diagnostics {
	p.configuredMu.Lock()
	defer p.configuredMu.Unlock()
	if p.configured {
		return common.Diagnostics{
			{
				Severity: common.Error,
				Summary:  "Provider already configured",
				Detail:   "This operation requires an unconfigured provider, but this provider was already configured.",
			},
		}
	}

	dv, diags := encodeDynamicValue(config.Value, p.schema.ProviderConfig)
	if diags.HasErrors() {
		return diags
	}
	resp, err := p.client.ConfigureProvider(ctx, &tfplugin6.ConfigureProvider_Request{
		Config: dv,
	})
	diags = append(diags, common.RPCErrorDiagnostics(err)...)
	if err != nil {
		return diags
	}
	diags = append(diags, decodeDiagnostics(resp.Diagnostics)...)
	if !diags.HasErrors() {
		p.configured = true
	}
	return diags
}

func (p *Provider) ValidateManagedResourceConfig(ctx context.Context, typeName string, config cty.Value) common.Diagnostics {
	dv, diags := encodeDynamicValue(config, p.schema.ProviderConfig)
	if diags.HasErrors() {
		return diags
	}
	resp, err := p.client.ValidateResourceConfig(ctx, &tfplugin6.ValidateResourceConfig_Request{
		Config: dv,
	})
	diags = append(diags, common.RPCErrorDiagnostics(err)...)
	if err != nil {
		return diags
	}
	diags = append(diags, decodeDiagnostics(resp.Diagnostics)...)
	return diags
}

func (p *Provider) ValidateDataResourceConfig(ctx context.Context, typeName string, config cty.Value) common.Diagnostics {
	dv, diags := encodeDynamicValue(config, p.schema.ProviderConfig)
	if diags.HasErrors() {
		return diags
	}
	resp, err := p.client.ValidateDataResourceConfig(ctx, &tfplugin6.ValidateDataResourceConfig_Request{
		Config: dv,
	})
	diags = append(diags, common.RPCErrorDiagnostics(err)...)
	if err != nil {
		return diags
	}
	diags = append(diags, decodeDiagnostics(resp.Diagnostics)...)
	return diags
}

func (p *Provider) ManagedResourceType(typeName string) common.ManagedResourceType {
	p.configuredMu.Lock()
	if !p.configured {
		return nil
	}
	p.configuredMu.Unlock()

	schema, ok := p.schema.ManagedResourceTypes[typeName]
	if !ok {
		return nil
	}
	return &ManagedResourceType{
		client:   p.client,
		typeName: typeName,
		schema:   schema,
	}
}

func (p *Provider) PlanResourceChange(ctx context.Context, req common.PlanResourceChangeRequest) (common.PlanResourceChangeResponse, common.Diagnostics) {
	var diags common.Diagnostics

	if moreDiags := p.requireConfigured(); moreDiags.HasErrors() {
		return common.PlanResourceChangeResponse{}, moreDiags
	}

	schema, ok := p.schema.ManagedResourceTypes[req.TypeName]
	if !ok {
		diags = append(diags, common.Diagnostic{
			Severity: common.Error,
			Summary:  "Invalid resource type",
			Detail:   fmt.Sprintf("The provider does not support resource type %q.", req.TypeName),
		})
		return common.PlanResourceChangeResponse{}, diags
	}

	priorDV, moreDiags := encodeDynamicValue(req.PriorState, schema.Content)
	diags = append(diags, moreDiags...)
	if moreDiags.HasErrors() {
		return common.PlanResourceChangeResponse{}, diags
	}

	proposedDV, moreDiags := encodeDynamicValue(req.ProposedNewState, schema.Content)
	diags = append(diags, moreDiags...)
	if moreDiags.HasErrors() {
		return common.PlanResourceChangeResponse{}, diags
	}

	configDV, moreDiags := encodeDynamicValue(req.Config, schema.Content)
	diags = append(diags, moreDiags...)
	if moreDiags.HasErrors() {
		return common.PlanResourceChangeResponse{}, diags
	}

	var providerMetaDV *tfplugin6.DynamicValue
	if !req.ProviderMeta.IsNull() {
		dv, moreDiags := encodeDynamicValue(req.ProviderMeta, p.schema.ProviderMeta)
		diags = append(diags, moreDiags...)
		if moreDiags.HasErrors() {
			return common.PlanResourceChangeResponse{}, diags
		}
		providerMetaDV = dv
	}

	resp, err := p.client.PlanResourceChange(ctx, &tfplugin6.PlanResourceChange_Request{
		TypeName:         req.TypeName,
		PriorState:       priorDV,
		ProposedNewState: proposedDV,
		Config:           configDV,
		PriorPrivate:     req.PriorPrivate,
		ProviderMeta:     providerMetaDV,
	})
	diags = append(diags, common.RPCErrorDiagnostics(err)...)
	if err != nil {
		return common.PlanResourceChangeResponse{}, diags
	}

	diags = append(diags, decodeDiagnostics(resp.Diagnostics)...)

	result := common.PlanResourceChangeResponse{
		PlannedPrivate: resp.PlannedPrivate,
	}

	if resp.PlannedState != nil {
		plannedState, moreDiags := decodeDynamicValue(resp.PlannedState, schema.Content)
		diags = append(diags, moreDiags...)
		result.PlannedState = plannedState
	}

	for _, attrPath := range resp.RequiresReplace {
		path := decodeAttributePath(attrPath)
		result.RequiresReplace = append(result.RequiresReplace, path)
	}

	return result, diags
}

func (p *Provider) ApplyResourceChange(ctx context.Context, req common.ApplyResourceChangeRequest) (common.ApplyResourceChangeResponse, common.Diagnostics) {
	var diags common.Diagnostics

	if moreDiags := p.requireConfigured(); moreDiags.HasErrors() {
		return common.ApplyResourceChangeResponse{}, moreDiags
	}

	schema, ok := p.schema.ManagedResourceTypes[req.TypeName]
	if !ok {
		diags = append(diags, common.Diagnostic{
			Severity: common.Error,
			Summary:  "Invalid resource type",
			Detail:   fmt.Sprintf("The provider does not support resource type %q.", req.TypeName),
		})
		return common.ApplyResourceChangeResponse{}, diags
	}

	priorDV, moreDiags := encodeDynamicValue(req.PriorState, schema.Content)
	diags = append(diags, moreDiags...)
	if moreDiags.HasErrors() {
		return common.ApplyResourceChangeResponse{}, diags
	}

	plannedDV, moreDiags := encodeDynamicValue(req.PlannedState, schema.Content)
	diags = append(diags, moreDiags...)
	if moreDiags.HasErrors() {
		return common.ApplyResourceChangeResponse{}, diags
	}

	configDV, moreDiags := encodeDynamicValue(req.Config, schema.Content)
	diags = append(diags, moreDiags...)
	if moreDiags.HasErrors() {
		return common.ApplyResourceChangeResponse{}, diags
	}

	var providerMetaDV *tfplugin6.DynamicValue
	if !req.ProviderMeta.IsNull() {
		dv, moreDiags := encodeDynamicValue(req.ProviderMeta, p.schema.ProviderMeta)
		diags = append(diags, moreDiags...)
		if moreDiags.HasErrors() {
			return common.ApplyResourceChangeResponse{}, diags
		}
		providerMetaDV = dv
	}

	resp, err := p.client.ApplyResourceChange(ctx, &tfplugin6.ApplyResourceChange_Request{
		TypeName:       req.TypeName,
		PriorState:     priorDV,
		PlannedState:   plannedDV,
		Config:         configDV,
		PlannedPrivate: req.PlannedPrivate,
		ProviderMeta:   providerMetaDV,
	})
	diags = append(diags, common.RPCErrorDiagnostics(err)...)
	if err != nil {
		return common.ApplyResourceChangeResponse{}, diags
	}

	diags = append(diags, decodeDiagnostics(resp.Diagnostics)...)

	result := common.ApplyResourceChangeResponse{
		Private: resp.Private,
	}

	if resp.NewState != nil {
		newState, moreDiags := decodeDynamicValue(resp.NewState, schema.Content)
		diags = append(diags, moreDiags...)
		result.NewState = newState
	}

	return result, diags
}

func (p *Provider) ReadResource(ctx context.Context, req common.ReadResourceRequest) (common.ReadResourceResponse, common.Diagnostics) {
	var diags common.Diagnostics

	if moreDiags := p.requireConfigured(); moreDiags.HasErrors() {
		return common.ReadResourceResponse{}, moreDiags
	}

	schema, ok := p.schema.ManagedResourceTypes[req.TypeName]
	if !ok {
		diags = append(diags, common.Diagnostic{
			Severity: common.Error,
			Summary:  "Invalid resource type",
			Detail:   fmt.Sprintf("The provider does not support resource type %q.", req.TypeName),
		})
		return common.ReadResourceResponse{}, diags
	}

	currentDV, moreDiags := encodeDynamicValue(req.CurrentState, schema.Content)
	diags = append(diags, moreDiags...)
	if moreDiags.HasErrors() {
		return common.ReadResourceResponse{}, diags
	}

	var providerMetaDV *tfplugin6.DynamicValue
	if !req.ProviderMeta.IsNull() {
		dv, moreDiags := encodeDynamicValue(req.ProviderMeta, p.schema.ProviderMeta)
		diags = append(diags, moreDiags...)
		if moreDiags.HasErrors() {
			return common.ReadResourceResponse{}, diags
		}
		providerMetaDV = dv
	}

	resp, err := p.client.ReadResource(ctx, &tfplugin6.ReadResource_Request{
		TypeName:     req.TypeName,
		CurrentState: currentDV,
		Private:      req.Private,
		ProviderMeta: providerMetaDV,
	})
	diags = append(diags, common.RPCErrorDiagnostics(err)...)
	if err != nil {
		return common.ReadResourceResponse{}, diags
	}

	diags = append(diags, decodeDiagnostics(resp.Diagnostics)...)

	result := common.ReadResourceResponse{
		Private: resp.Private,
	}

	if resp.NewState != nil {
		newState, moreDiags := decodeDynamicValue(resp.NewState, schema.Content)
		diags = append(diags, moreDiags...)
		result.NewState = newState
	}

	return result, diags
}

func (p *Provider) ImportResourceState(ctx context.Context, req common.ImportResourceStateRequest) (common.ImportResourceStateResponse, common.Diagnostics) {
	var diags common.Diagnostics

	if moreDiags := p.requireConfigured(); moreDiags.HasErrors() {
		return common.ImportResourceStateResponse{}, moreDiags
	}

	schema, ok := p.schema.ManagedResourceTypes[req.TypeName]
	if !ok {
		diags = append(diags, common.Diagnostic{
			Severity: common.Error,
			Summary:  "Invalid resource type",
			Detail:   fmt.Sprintf("The provider does not support resource type %q.", req.TypeName),
		})
		return common.ImportResourceStateResponse{}, diags
	}

	resp, err := p.client.ImportResourceState(ctx, &tfplugin6.ImportResourceState_Request{
		TypeName: req.TypeName,
		Id:       req.ID,
	})
	diags = append(diags, common.RPCErrorDiagnostics(err)...)
	if err != nil {
		return common.ImportResourceStateResponse{}, diags
	}

	diags = append(diags, decodeDiagnostics(resp.Diagnostics)...)

	result := common.ImportResourceStateResponse{}

	for _, imported := range resp.ImportedResources {
		importedSchema := schema
		if imported.TypeName != req.TypeName {
			// Check if the returned type name is valid
			if altSchema, ok := p.schema.ManagedResourceTypes[imported.TypeName]; ok {
				importedSchema = altSchema
			} else {
				diags = append(diags, common.Diagnostic{
					Severity: common.Error,
					Summary:  "Invalid imported resource type",
					Detail:   fmt.Sprintf("The provider returned an invalid resource type %q during import.", imported.TypeName),
				})
				continue
			}
		}

		state, moreDiags := decodeDynamicValue(imported.State, importedSchema.Content)
		diags = append(diags, moreDiags...)
		if !moreDiags.HasErrors() {
			result.ImportedResources = append(result.ImportedResources, common.ImportedResource{
				TypeName: imported.TypeName,
				State:    state,
				Private:  imported.Private,
			})
		}
	}

	return result, diags
}

func (p *Provider) Close() error {
	return p.plugin.Close()
}

func (p *Provider) requireConfigured() common.Diagnostics {
	p.configuredMu.Lock()
	var diags common.Diagnostics
	if !p.configured {
		diags = append(diags, common.Diagnostic{
			Severity: common.Error,
			Summary:  "Provider unconfigured",
			Detail:   "This operation requires a configured provider, but the provider isn't configured yet.",
		})
	}
	p.configuredMu.Unlock()
	return diags
}
