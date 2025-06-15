package api

import (
	"context"
	"fmt"

	pb "github.com/atticplaygroup/pkv/pkg/proto/gen/go/kvstore"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
)

func (s *Server) CreateValue(ctx context.Context, req *pb.CreateValueRequest) (*pb.CreateValueResponse, error) {
	name := fmt.Sprintf("%s/values/%s", req.GetParent(), uuid.New().String())
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
		ret := pb.CreateValueResponse{
			Name: name,
			Ttl:  req.GetTtl(),
		}
		return &ret, nil
	}
}

func (s *Server) GetValue(ctx context.Context, req *pb.GetValueRequest) (*pb.GetValueResponse, error) {
	value, err := s.redisClient.Get(ctx, req.GetName()).Result()
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
		ret := pb.GetValueResponse{
			Value: []byte(value),
		}
		return &ret, nil
	}
}

func (s *Server) ProlongValue(ctx context.Context, req *pb.ProlongValueRequest) (*pb.ProlongValueResponse, error) {
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
		ret := pb.ProlongValueResponse{
			Name: req.GetName(),
			Ttl:  durationpb.New(newTtl),
		}
		return &ret, nil
	}
}
