package protocol5

import (
	"context"

	"github.com/apparentlymart/terraform-provider/internal/tfplugin5"
	"github.com/apparentlymart/terraform-provider/tfprovider/internal/common"
	"github.com/apparentlymart/terraform-schema-go/tfschema"
	"github.com/zclconf/go-cty/cty"
)

type DataResourceType struct {
	client             tfplugin5.ProviderClient
	typeName           string
	schema             *common.DataResourceTypeSchema
	providerMetaSchema *tfschema.Block
}

func (rt *DataResourceType) ValidateConfig(ctx context.Context, config cty.Value) common.Diagnostics {
	dv, diags := encodeDynamicValue(config, rt.schema.Content)
	if diags.HasErrors() {
		return diags
	}
	resp, err := rt.client.ValidateDataSourceConfig(ctx, &tfplugin5.ValidateDataSourceConfig_Request{
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

func (rt *DataResourceType) Read(ctx context.Context, req common.DataResourceReadRequest) (common.DataResourceReadResponse, common.Diagnostics) {
	var diags common.Diagnostics

	configDV, moreDiags := encodeDynamicValue(req.Config, rt.schema.Content)
	diags = append(diags, moreDiags...)
	if moreDiags.HasErrors() {
		return common.DataResourceReadResponse{}, diags
	}

	var providerMetaDV *tfplugin5.DynamicValue
	if !req.ProviderMeta.IsNull() && rt.providerMetaSchema != nil {
		var moreDiags common.Diagnostics
		providerMetaDV, moreDiags = encodeDynamicValue(req.ProviderMeta, rt.providerMetaSchema)
		diags = append(diags, moreDiags...)
		if moreDiags.HasErrors() {
			return common.DataResourceReadResponse{}, diags
		}
	}

	resp, err := rt.client.ReadDataSource(ctx, &tfplugin5.ReadDataSource_Request{
		TypeName:     rt.typeName,
		Config:       configDV,
		ProviderMeta: providerMetaDV,
	})
	diags = append(diags, common.RPCErrorDiagnostics(err)...)
	if err != nil {
		return common.DataResourceReadResponse{}, diags
	}

	diags = append(diags, decodeDiagnostics(resp.Diagnostics)...)

	result := common.DataResourceReadResponse{}

	if resp.State != nil {
		state, moreDiags := decodeDynamicValue(resp.State, rt.schema.Content)
		diags = append(diags, moreDiags...)
		result.State = state
	}

	return result, diags
}

func (rt *DataResourceType) Sealed() common.Sealed {
	return common.Sealed{}
}