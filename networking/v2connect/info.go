package v2connect

import (
	"context"

	"github.com/setavenger/blindbit-lib/proto/pb"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (c *OracleClient) GetInfo(ctx context.Context) (*pb.InfoResponse, error) {
	return c.client.GetInfo(ctx, &emptypb.Empty{})
}
