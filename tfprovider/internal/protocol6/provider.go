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
		return common.Config{Value: config}, diags
	}
	return common.Config{Value: config}, diags
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


func (p *Provider) ManagedResourceType(typeName string) (common.ManagedResourceType, error) {
	p.configuredMu.Lock()
	if !p.configured {
		p.configuredMu.Unlock()
		return nil, fmt.Errorf("provider not configured")
	}
	p.configuredMu.Unlock()

	schema, ok := p.schema.ManagedResourceTypes[typeName]
	if !ok {
		return nil, fmt.Errorf("managed resource type %q not found", typeName)
	}
	return &ManagedResourceType{
		client:             p.client,
		typeName:           typeName,
		schema:             schema,
		providerMetaSchema: p.schema.ProviderMeta,
	}, nil
}

func (p *Provider) DataResourceType(typeName string) (common.DataResourceType, error) {
	p.configuredMu.Lock()
	if !p.configured {
		p.configuredMu.Unlock()
		return nil, fmt.Errorf("provider not configured")
	}
	p.configuredMu.Unlock()

	schema, ok := p.schema.DataResourceTypes[typeName]
	if !ok {
		return nil, fmt.Errorf("data resource type %q not found", typeName)
	}
	return &DataResourceType{
		client:             p.client,
		typeName:           typeName,
		schema:             schema,
		providerMetaSchema: p.schema.ProviderMeta,
	}, nil
}

func (p *Provider) Close() error {
	return p.plugin.Close()
}
