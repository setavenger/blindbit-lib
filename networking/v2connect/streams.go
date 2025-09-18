package v2connect

import (
	"context"

	"github.com/setavenger/blindbit-lib/proto/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type OracleClient struct {
	client pb.OracleServiceClient
	conn   *grpc.ClientConn
}

func NewClient(ctx context.Context, address string) (*OracleClient, error) {
	conn, err := grpc.NewClient(
		address, grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, err
	}
	return &OracleClient{
		client: pb.NewOracleServiceClient(conn),
		conn:   conn,
	}, nil
}

func (c *OracleClient) Close() error {
	return c.conn.Close()
}

func (c *OracleClient) StreamBlockBatchFull(
	ctx context.Context, request *pb.RangedBlockHeightRequest,
) (
	pb.OracleService_StreamBlockBatchFullClient, error,
) {
	return c.client.StreamBlockBatchFull(ctx, request)
}

func (c *OracleClient) StreamBlockBatchFullStatic(
	ctx context.Context, request *pb.RangedBlockHeightRequest,
) (
	pb.OracleService_StreamBlockBatchFullStaticClient, error,
) {
	return c.client.StreamBlockBatchFullStatic(ctx, request)
}

func (c *OracleClient) StreamIndexShortOuts(
	ctx context.Context, request *pb.RangedBlockHeightRequestFiltered,
) (
	pb.OracleService_StreamIndexShortOutsClient, error,
) {
	return c.client.StreamIndexShortOuts(ctx, request)
}
