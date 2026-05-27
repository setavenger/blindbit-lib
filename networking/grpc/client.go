package grpc

import (
	"context"
	"crypto/tls"

	"github.com/setavenger/blindbit-lib/proto/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
)

type OracleClient struct {
	client pb.OracleServiceClient
	conn   *grpc.ClientConn
}

func NewClient(
	ctx context.Context, address string, useTLS bool,
) (
	*OracleClient, error,
) {
	var tc credentials.TransportCredentials
	if useTLS {
		tc = credentials.NewTLS(&tls.Config{})
	} else {
		tc = insecure.NewCredentials()
	}

	var opts []grpc.DialOption
	opts = append(opts, grpc.WithTransportCredentials(tc))

	conn, err := grpc.NewClient(
		address, opts...,
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

func (c *OracleClient) GetInfo(
	ctx context.Context,
) (
	*pb.InfoResponse, error,
) {
	return c.client.GetInfo(ctx, &emptypb.Empty{})
}

func (c *OracleClient) GetSpentOutputsShort(
	ctx context.Context, request *pb.BlockHeightRequest,
) (
	*pb.IndexResponse, error,
) {
	return c.client.GetSpentOutputsShort(ctx, request)
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
