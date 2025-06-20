package protocol5

import (
	"context"
	"fmt"

	"github.com/apparentlymart/terraform-schema-go/tfschema"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"

	"github.com/apparentlymart/terraform-provider/internal/tfplugin5"
	"github.com/apparentlymart/terraform-provider/tfprovider/internal/common"
)

func decodeProviderSchemaBlock(raw *tfplugin5.Schema_Block) *tfschema.Block {
	var ret tfschema.Block
	if raw == nil {
		return &ret
	}

	ret.Attributes = make(map[string]*tfschema.Attribute)
	ret.BlockTypes = make(map[string]*tfschema.NestedBlock)

	for _, rawAttr := range raw.Attributes {
		rawType := rawAttr.Type
		ty, err := ctyjson.UnmarshalType(rawType)
		if err != nil {
			// If the provider sends us an invalid type then we'll just
			// replace it with dynamic, since the provider is misbehaving.
			ty = cty.DynamicPseudoType
		}

		ret.Attributes[rawAttr.Name] = &tfschema.Attribute{
			Type:        ty,
			Description: rawAttr.Description,

			Required:  rawAttr.Required,
			Optional:  rawAttr.Optional,
			Computed:  rawAttr.Computed,
			Sensitive: rawAttr.Sensitive,
		}
	}

	for _, rawBlock := range raw.BlockTypes {
		var mode tfschema.NestingMode
		switch rawBlock.Nesting {
		case tfplugin5.Schema_NestedBlock_SINGLE:
			mode = tfschema.NestingSingle
		case tfplugin5.Schema_NestedBlock_GROUP:
			mode = tfschema.NestingGroup
		case tfplugin5.Schema_NestedBlock_LIST:
			mode = tfschema.NestingList
		case tfplugin5.Schema_NestedBlock_SET:
			mode = tfschema.NestingSet
		case tfplugin5.Schema_NestedBlock_MAP:
			mode = tfschema.NestingMap
		}

		content := decodeProviderSchemaBlock(rawBlock.Block)

		ret.BlockTypes[rawBlock.TypeName] = &tfschema.NestedBlock{
			Nesting: mode,
			Block:   *content,
		}
	}

	return &ret
}

func loadSchema(ctx context.Context, client tfplugin5.ProviderClient) (*common.Schema, error) {
	resp, err := client.GetSchema(ctx, &tfplugin5.GetProviderSchema_Request{})
	if err != nil {
		return nil, err
	}
	diags := decodeDiagnostics(resp.Diagnostics)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to retrieve provider schema")
	}
	var ret common.Schema
	ret.ProviderConfig = decodeProviderSchemaBlock(resp.Provider.Block)
	ret.ProviderMeta = decodeProviderSchemaBlock(resp.ProviderMeta.Block)
	ret.ManagedResourceTypes = make(map[string]*common.ManagedResourceTypeSchema)
	for name, raw := range resp.ResourceSchemas {
		ret.ManagedResourceTypes[name] = &common.ManagedResourceTypeSchema{
			Version: raw.Version,
			Content: decodeProviderSchemaBlock(raw.Block),
		}
	}
	ret.DataResourceTypes = make(map[string]*common.DataResourceTypeSchema)
	for name, raw := range resp.DataSourceSchemas {
		ret.DataResourceTypes[name] = &common.DataResourceTypeSchema{
			Content: decodeProviderSchemaBlock(raw.Block),
		}
	}
	return &ret, nil
}

func encodeDynamicValue(val cty.Value, schema *tfschema.Block) (*tfplugin5.DynamicValue, common.Diagnostics) {
	data, diags := common.EncodeDynamicValue(val, schema)
	if diags.HasErrors() {
		return nil, diags
	}
	return &tfplugin5.DynamicValue{
		Json:    data.JSON,
		Msgpack: data.Msgpack,
	}, nil
}

func decodeDynamicValue(raw *tfplugin5.DynamicValue, schema *tfschema.Block) (cty.Value, common.Diagnostics) {
	data := common.DynamicValueData{
		JSON:    raw.Json,
		Msgpack: raw.Msgpack,
	}
	return common.DecodeDynamicValue(data, schema)
}
