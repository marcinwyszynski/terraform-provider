package protocol6

import (
	"github.com/zclconf/go-cty/cty"

	"github.com/apparentlymart/terraform-provider/internal/tfplugin6"
	"github.com/apparentlymart/terraform-provider/tfprovider/internal/common"
)

func decodeDiagnostics(raws []*tfplugin6.Diagnostic) common.Diagnostics {
	if len(raws) == 0 {
		return nil
	}
	diags := make(common.Diagnostics, 0, len(raws))
	for _, raw := range raws {
		diag := common.Diagnostic{
			Summary:   raw.Summary,
			Detail:    raw.Detail,
			Attribute: decodeAttributePath(raw.Attribute),
		}

		switch raw.Severity {
		case tfplugin6.Diagnostic_ERROR:
			diag.Severity = common.Error
		case tfplugin6.Diagnostic_WARNING:
			diag.Severity = common.Warning
		}

		diags = append(diags, diag)
	}
	return diags
}

func decodeAttributePath(raws *tfplugin6.AttributePath) cty.Path {
	if raws == nil || len(raws.Steps) == 0 {
		return nil
	}
	ret := make(cty.Path, 0, len(raws.Steps))
	for _, raw := range raws.Steps {
		switch s := raw.GetSelector().(type) {
		case *tfplugin6.AttributePath_Step_AttributeName:
			ret = ret.GetAttr(s.AttributeName)
		case *tfplugin6.AttributePath_Step_ElementKeyString:
			ret = ret.Index(cty.StringVal(s.ElementKeyString))
		case *tfplugin6.AttributePath_Step_ElementKeyInt:
			ret = ret.Index(cty.NumberIntVal(s.ElementKeyInt))
		default:
			ret = append(ret, nil)
		}
	}
	return ret
}
