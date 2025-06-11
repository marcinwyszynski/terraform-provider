package common

import (
	"github.com/apparentlymart/terraform-schema-go/tfschema"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/json"
	"github.com/zclconf/go-cty/cty/msgpack"
)

// DynamicValueData represents the raw bytes and format for a dynamic value
type DynamicValueData struct {
	JSON    []byte
	Msgpack []byte
}

// EncodeDynamicValue encodes a cty.Value into msgpack format
func EncodeDynamicValue(val cty.Value, schema *tfschema.Block) (DynamicValueData, Diagnostics) {
	ty := schema.ImpliedType()
	raw, err := msgpack.Marshal(val, ty)
	if err != nil {
		return DynamicValueData{}, ErrorDiagnostics(
			"Invalid object",
			"Value does not have the required type",
			err,
		)
	}
	return DynamicValueData{
		Msgpack: raw,
	}, nil
}

// DecodeDynamicValue decodes raw dynamic value data back into a cty.Value
func DecodeDynamicValue(data DynamicValueData, schema *tfschema.Block) (cty.Value, Diagnostics) {
	ty := schema.ImpliedType()
	switch {
	case len(data.JSON) > 0:
		val, err := json.Unmarshal(data.JSON, ty)
		if err != nil {
			return cty.DynamicVal, ErrorDiagnostics(
				"Provider returned invalid object",
				"Provider's JSON response does not conform to the expected type",
				err,
			)
		}
		return val, nil
	case len(data.Msgpack) > 0:
		val, err := msgpack.Unmarshal(data.Msgpack, ty)
		if err != nil {
			return cty.DynamicVal, ErrorDiagnostics(
				"Provider returned invalid object",
				"Provider's msgpack response does not conform to the expected type",
				err,
			)
		}
		return val, nil
	default:
		return cty.DynamicVal, Diagnostics{
			{
				Severity: Error,
				Summary:  "Provider using unsupported response format",
				Detail:   "Provider's response is not in either JSON or msgpack format",
			},
		}
	}
}