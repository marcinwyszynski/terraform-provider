package protocol6

import (
	"context"

	"google.golang.org/grpc"

	"github.com/apparentlymart/terraform-provider/internal/tfplugin6"
)

type PluginClient struct{}

func (c PluginClient) ClientProxy(ctx context.Context, conn *grpc.ClientConn) (interface{}, error) {
	return tfplugin6.NewProviderClient(conn), nil
}
