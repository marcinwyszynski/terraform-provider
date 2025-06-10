package protocol5

import (
	"context"
	"fmt"
	"sync"

	"github.com/apparentlymart/terraform-provider/internal/tfplugin5"
	"github.com/apparentlymart/terraform-provider/tfprovider/internal/common"
	"github.com/zclconf/go-cty/cty"
	"go.rpcplugin.org/rpcplugin"
)

// Provider is the implementation of tfprovider.Provider for provider plugin
// protocol version 5.
type Provider struct {
	client tfplugin5.ProviderClient
	plugin *rpcplugin.Plugin
	schema *common.Schema

	configured   bool
	configuredMu *sync.Mutex
}

func NewProvider(ctx context.Context, plugin *rpcplugin.Plugin, clientProxy interface{}) (*Provider, error) {
	client := clientProxy.(tfplugin5.ProviderClient)

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
	dv, diags := encodeDynamicValue(config, p.schema.ProviderConfig)
	if diags.HasErrors() {
		return common.Config{config}, diags
	}
	resp, err := p.client.PrepareProviderConfig(ctx, &tfplugin5.PrepareProviderConfig_Request{
		Config: dv,
	})
	diags = append(diags, common.RPCErrorDiagnostics(err)...)
	if err != nil {
		return common.Config{config}, diags
	}
	diags = append(diags, decodeDiagnostics(resp.Diagnostics)...)
	if raw := resp.PreparedConfig; raw != nil {
		v, moreDiags := decodeDynamicValue(raw, p.schema.ProviderConfig)
		diags = append(diags, moreDiags...)
		return common.Config{v}, diags
	}
	return common.Config{cty.DynamicVal}, diags
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
	resp, err := p.client.Configure(ctx, &tfplugin5.Configure_Request{
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
	resp, err := p.client.ValidateResourceTypeConfig(ctx, &tfplugin5.ValidateResourceTypeConfig_Request{
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
	resp, err := p.client.ValidateDataSourceConfig(ctx, &tfplugin5.ValidateDataSourceConfig_Request{
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

	schema, moreDiags := p.commonResourceValidation(req.TypeName)
	if moreDiags.HasErrors() {
		return common.PlanResourceChangeResponse{}, moreDiags
	}

	values, moreDiags := p.encodePlanValues(schema, req.PriorState, req.ProposedNewState, req.Config)
	diags = append(diags, moreDiags...)
	if moreDiags.HasErrors() {
		return common.PlanResourceChangeResponse{}, diags
	}

	providerMetaDV, moreDiags := p.encodeProviderMeta(req.ProviderMeta)
	diags = append(diags, moreDiags...)
	if moreDiags.HasErrors() {
		return common.PlanResourceChangeResponse{}, diags
	}

	resp, err := p.client.PlanResourceChange(ctx, &tfplugin5.PlanResourceChange_Request{
		TypeName:         req.TypeName,
		PriorState:       values.Prior,
		ProposedNewState: values.Proposed,
		Config:           values.Config,
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

	schema, moreDiags := p.commonResourceValidation(req.TypeName)
	if moreDiags.HasErrors() {
		return common.ApplyResourceChangeResponse{}, moreDiags
	}

	values, moreDiags := p.encodeApplyValues(schema, req.PriorState, req.PlannedState, req.Config)
	diags = append(diags, moreDiags...)
	if moreDiags.HasErrors() {
		return common.ApplyResourceChangeResponse{}, diags
	}

	providerMetaDV, moreDiags := p.encodeProviderMeta(req.ProviderMeta)
	diags = append(diags, moreDiags...)
	if moreDiags.HasErrors() {
		return common.ApplyResourceChangeResponse{}, diags
	}

	resp, err := p.client.ApplyResourceChange(ctx, &tfplugin5.ApplyResourceChange_Request{
		TypeName:       req.TypeName,
		PriorState:     values.Prior,
		PlannedState:   values.Planned,
		Config:         values.Config,
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

	schema, moreDiags := p.commonResourceValidation(req.TypeName)
	if moreDiags.HasErrors() {
		return common.ReadResourceResponse{}, moreDiags
	}

	values, moreDiags := p.encodeCurrentValue(schema, req.CurrentState)
	diags = append(diags, moreDiags...)
	if moreDiags.HasErrors() {
		return common.ReadResourceResponse{}, diags
	}

	providerMetaDV, moreDiags := p.encodeProviderMeta(req.ProviderMeta)
	diags = append(diags, moreDiags...)
	if moreDiags.HasErrors() {
		return common.ReadResourceResponse{}, diags
	}

	resp, err := p.client.ReadResource(ctx, &tfplugin5.ReadResource_Request{
		TypeName:     req.TypeName,
		CurrentState: values.Current,
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

	schema, moreDiags := p.commonResourceValidation(req.TypeName)
	if moreDiags.HasErrors() {
		return common.ImportResourceStateResponse{}, moreDiags
	}

	resp, err := p.client.ImportResourceState(ctx, &tfplugin5.ImportResourceState_Request{
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

// validateResourceType checks if the given resource type is supported and returns its schema
func (p *Provider) validateResourceType(typeName string) (*common.ManagedResourceTypeSchema, common.Diagnostics) {
	schema, ok := p.schema.ManagedResourceTypes[typeName]
	if !ok {
		return nil, common.Diagnostics{
			{
				Severity: common.Error,
				Summary:  "Invalid resource type",
				Detail:   fmt.Sprintf("The provider does not support resource type %q.", typeName),
			},
		}
	}
	return schema, nil
}

// encodeProviderMeta encodes the provider metadata if present
func (p *Provider) encodeProviderMeta(providerMeta cty.Value) (*tfplugin5.DynamicValue, common.Diagnostics) {
	if providerMeta.IsNull() {
		return nil, nil
	}
	return encodeDynamicValue(providerMeta, p.schema.ProviderMeta)
}

// commonResourceValidation performs the common validation steps for resource operations
func (p *Provider) commonResourceValidation(typeName string) (*common.ManagedResourceTypeSchema, common.Diagnostics) {
	if diags := p.requireConfigured(); diags.HasErrors() {
		return nil, diags
	}
	return p.validateResourceType(typeName)
}

// EncodedValues holds encoded dynamic values for resource operations
type EncodedValues struct {
	Prior     *tfplugin5.DynamicValue
	Proposed  *tfplugin5.DynamicValue
	Planned   *tfplugin5.DynamicValue
	Config    *tfplugin5.DynamicValue
	Current   *tfplugin5.DynamicValue
}

// encodePlanValues encodes the values needed for PlanResourceChange
func (p *Provider) encodePlanValues(schema *common.ManagedResourceTypeSchema, prior, proposed, config cty.Value) (EncodedValues, common.Diagnostics) {
	var diags common.Diagnostics
	var result EncodedValues
	
	var err common.Diagnostics
	result.Prior, err = encodeDynamicValue(prior, schema.Content)
	diags = append(diags, err...)
	if err.HasErrors() {
		return result, diags
	}
	
	result.Proposed, err = encodeDynamicValue(proposed, schema.Content)
	diags = append(diags, err...)
	if err.HasErrors() {
		return result, diags
	}
	
	result.Config, err = encodeDynamicValue(config, schema.Content)
	diags = append(diags, err...)
	return result, diags
}

// encodeApplyValues encodes the values needed for ApplyResourceChange
func (p *Provider) encodeApplyValues(schema *common.ManagedResourceTypeSchema, prior, planned, config cty.Value) (EncodedValues, common.Diagnostics) {
	var diags common.Diagnostics
	var result EncodedValues
	
	var err common.Diagnostics
	result.Prior, err = encodeDynamicValue(prior, schema.Content)
	diags = append(diags, err...)
	if err.HasErrors() {
		return result, diags
	}
	
	result.Planned, err = encodeDynamicValue(planned, schema.Content)
	diags = append(diags, err...)
	if err.HasErrors() {
		return result, diags
	}
	
	result.Config, err = encodeDynamicValue(config, schema.Content)
	diags = append(diags, err...)
	return result, diags
}

// encodeCurrentValue encodes a single current state value for ReadResource
func (p *Provider) encodeCurrentValue(schema *common.ManagedResourceTypeSchema, current cty.Value) (EncodedValues, common.Diagnostics) {
	var result EncodedValues
	var diags common.Diagnostics
	
	result.Current, diags = encodeDynamicValue(current, schema.Content)
	return result, diags
}
