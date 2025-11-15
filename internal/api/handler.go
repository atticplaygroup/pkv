package api

import (
	"context"
	"fmt"
	"strings"

	"bytes"
	"crypto/sha256"
	"io"

	"connectrpc.com/connect"
	pb "github.com/atticplaygroup/pkv/pkg/proto/gen/go/kvstore/v1"
	"github.com/ipfs/boxo/ipld/merkledag"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multicodec"
	"github.com/multiformats/go-multihash"
)

func HashRawBytes(inputBytes []byte) string {
	h := sha256.New()
	io.Copy(h, bytes.NewReader(inputBytes))
	digest := h.Sum(nil)
	mh, err := multihash.Encode(digest, multihash.SHA2_256)
	if err != nil {
		panic(err)
	}
	return cid.NewCidV1(uint64(multicodec.Raw), mh).String()
}

func (s *Server) Ping(
	ctx context.Context,
	connectReq *connect.Request[pb.PingRequest],
) (*connect.Response[pb.PingResponse], error) {
	return connect.NewResponse(&pb.PingResponse{
		Pong: "pong",
	}), nil
}

func calculateCid(req *pb.CreateValueRequest) (string, error) {
	switch req.GetCodec() {
	case pb.CreateValueRequest_CODEC_DAG_PB:
		protoNode, err := merkledag.DecodeProtobuf(req.GetValue())
		if err != nil {
			return "", err
		}
		if protoNode.Cid().Prefix().Codec != cid.DagProtobuf {
			return "", fmt.Errorf(
				"expected codec to be dag-pb (%d) but got %d",
				cid.DagProtobuf,
				protoNode.Cid().Prefix().Codec,
			)
		}
		return cid.NewCidV1(cid.DagProtobuf, protoNode.Cid().Hash()).String(), nil
	case pb.CreateValueRequest_CODEC_RAW:
		return HashRawBytes(req.GetValue()), nil
	default:
		return "", fmt.Errorf("only raw and dag-pb codec are allowed")
	}
}

func (s *Server) CreateValue(
	ctx context.Context, connectReq *connect.Request[pb.CreateValueRequest],
) (*connect.Response[pb.CreateValueResponse], error) {
	req := connectReq.Msg
	cid, err := calculateCid(req)
	if err != nil {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"failed to calculate cid: %s",
			err.Error(),
		)
	}
	// TODO: if cid turns out to be existing then prolong the ttl
	name := fmt.Sprintf("values/%s", cid)
	if err := s.redisClient.Set(
		ctx,
		name,
		req.GetValue(),
		req.GetTtl().AsDuration(),
	).Err(); err != nil {
		return nil, status.Error(
			codes.Internal,
			"failed to set value",
		)
	} else {
		return connect.NewResponse(&pb.CreateValueResponse{
			Name: name,
			Ttl:  req.GetTtl(),
		}), nil
	}
}

func (s *Server) GetValue(
	ctx context.Context, connectReq *connect.Request[pb.GetValueRequest],
) (*connect.Response[pb.GetValueResponse], error) {
	req := connectReq.Msg
	cid, find := strings.CutPrefix(req.GetName(), "values/")
	if !find {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"expected resource name begin with \"values/\" but got %s",
			req.GetName(),
		)
	}
	cidV1, err := NormalizeCidToV1(cid)
	if err != nil {
		return nil, err
	}
	cidKey := fmt.Sprintf("values/%s", cidV1)
	value, err := s.redisClient.Get(ctx, cidKey).Result()
	if err == redis.Nil {
		return nil, status.Error(
			codes.NotFound,
			"resource not found",
		)
	} else if err != nil {
		return nil, status.Error(
			codes.Internal,
			"failed to get value",
		)
	} else {
		return connect.NewResponse(&pb.GetValueResponse{
			Value: []byte(value),
		}), nil
	}
}

func (s *Server) ProlongValue(
	ctx context.Context, connectReq *connect.Request[pb.ProlongValueRequest],
) (*connect.Response[pb.ProlongValueResponse], error) {
	req := connectReq.Msg
	if req.GetTtl().AsDuration() <= 0 {
		return nil, status.Error(
			codes.InvalidArgument,
			"ttl and/or maxSize missing or corrupted",
		)
	}
	valueSize, err := s.redisClient.MemoryUsage(ctx, req.GetName()).Result()
	if err == redis.Nil {
		return nil, status.Error(
			codes.NotFound,
			"resource not found",
		)
	} else if err != nil {
		return nil, status.Error(
			codes.Internal,
			"failed to check memory usage",
		)
	} else if valueSize > req.GetMaxSize() {
		return nil, status.Error(
			codes.PermissionDenied,
			"value size exceeded",
		)
	}
	oldTtl, err := s.redisClient.TTL(ctx, req.GetName()).Result()
	if err == redis.Nil {
		return nil, status.Error(
			codes.NotFound,
			"resource not found",
		)
	} else if err != nil {
		return nil, status.Error(
			codes.Internal,
			"failed to get ttl",
		)
	}
	newTtl := req.GetTtl().AsDuration() + oldTtl
	if err = s.redisClient.Expire(ctx, req.GetName(), newTtl).Err(); err != nil {
		return nil, status.Error(
			codes.Internal,
			"failed to set ttl",
		)
	} else {
		return connect.NewResponse(&pb.ProlongValueResponse{
			Name: req.GetName(),
			Ttl:  durationpb.New(newTtl),
		}), nil
	}
}
