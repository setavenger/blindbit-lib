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

func (c *OracleClient) GetFullBlock(
	ctx context.Context, request *pb.BlockHeightRequest,
) (*pb.FullBlockResponse, error) {
	return c.client.GetFullBlock(ctx, request)
}

func (c *OracleClient) StreamComputeIndex(
	ctx context.Context, request *pb.RangedBlockHeightRequestFiltered,
) (
	pb.OracleService_StreamComputeIndexClient, error,
) {
	return c.client.StreamComputeIndex(ctx, request)
}

func (c *OracleClient) StreamBlockScanDataShort(
	ctx context.Context, request *pb.RangedBlockHeightRequestFiltered,
) (
	pb.OracleService_StreamBlockScanDataShortClient, error,
) {
	return c.client.StreamBlockScanDataShort(ctx, request)
}
