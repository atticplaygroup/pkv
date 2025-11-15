package api

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"sort"

	"connectrpc.com/connect"
	pb "github.com/atticplaygroup/pkv/pkg/proto/gen/go/kvstore/v1"
	"github.com/ipfs/go-cid"
	"github.com/mr-tron/base58/base58"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) RegisterInstance(
	ctx context.Context, connectReq *connect.Request[pb.RegisterInstanceRequest],
) (*connect.Response[pb.RegisterInstanceResponse], error) {
	req := connectReq.Msg
	processors := []MerkleTreeFileServing{NewServeAllFileServing(s.redisClient)}
	for _, p := range processors {
		matching, err := p.IsBehaviorMatching(req.GetAdvertisement().GetVirtualService().GetBehaviorLink())
		if err != nil {
			return nil, status.Errorf(
				codes.InvalidArgument,
				"behavior link not matching: %s",
				err.Error(),
			)
		}
		if !matching {
			continue
		}
		if err := p.EnsureAdvertisementValid(req.GetAdvertisement()); err != nil {
			return nil, status.Errorf(
				codes.InvalidArgument,
				"advertisement is not valid: %s",
				err.Error(),
			)
		}
		if err := p.Register(ctx, req.GetAdvertisement()); err != nil {
			return nil, status.Errorf(
				codes.Internal,
				"failed to register: %s",
				err.Error(),
			)
		}
		return &connect.Response[pb.RegisterInstanceResponse]{}, nil
	}
	return nil, status.Error(
		codes.InvalidArgument,
		"no processor matches required behavior",
	)
}

type AdWithScore struct {
	score float64
	ad    *pb.ProviderAdvertise
}

func (s *Server) searchInstances(
	ctx context.Context,
	vsvc *pb.VirtualService,
	instanceCount int64,
) ([]*AdWithScore, error) {
	instances := make([]*AdWithScore, 0, instanceCount)
	vHash, err := HashMessage(vsvc)
	if err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"failed to hash virtual service: %s",
			err.Error(),
		)
	}
	instanceKey := fmt.Sprintf("vsvc:instance:%v", vHash)
	zs, err := s.redisClient.ZRangeWithScores(
		ctx, instanceKey, 0, instanceCount,
	).Result()
	if err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"failed to get instances: %s",
			err.Error(),
		)
	}
	for _, z := range zs {
		instanceKey := fmt.Sprintf("instance:%s", z.Member.(string))
		var ad pb.ProviderAdvertise
		if err = s.redisClient.Get(ctx, instanceKey).Scan(&ad); err != nil {
			fmt.Printf("invalid instance advertisement from redis: %s\n", err)
			continue
		}
		instances = append(instances, &AdWithScore{
			score: z.Score,
			ad:    &ad,
		})
		if len(instances) == int(instanceCount) {
			return instances, nil
		}
	}
	return instances, nil
}

func (s *Server) SearchInstance(
	ctx context.Context,
	connectReq *connect.Request[pb.SearchInstanceRequest],
) (*connect.Response[pb.SearchInstanceResponse], error) {
	req := connectReq.Msg
	defaultInstanceCount := int64(100)
	instances := make([]*pb.ProviderAdvertise, 0, defaultInstanceCount)
	batchInstances, err := s.searchInstances(
		ctx, req.GetVirtualService(), defaultInstanceCount,
	)
	for _, i := range batchInstances {
		instances = append(instances, i.ad)
	}
	if err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"failed to search instance: %s",
			err.Error(),
		)
	}
	return connect.NewResponse(&pb.SearchInstanceResponse{
		VirtualService:    req.GetVirtualService(),
		InstancePriceInfo: instances,
	}), nil
}

func (s *Server) SearchCid(
	ctx context.Context, connectReq *connect.Request[pb.SearchCidRequest],
) (*connect.Response[pb.SearchCidResponse], error) {
	return s.doSearchCid(ctx, connectReq)
}

func NormalizeCidToV1(cidString string) (string, error) {
	requestCid, err := cid.Decode(cidString)
	if err != nil {
		return "", status.Errorf(
			codes.InvalidArgument,
			"failed to decode cid: %s",
			err.Error(),
		)
	}
	codec := requestCid.Prefix().Codec
	if requestCid.Version() == 1 && (codec != cid.Raw && codec != cid.DagProtobuf) {
		return "", status.Errorf(
			codes.InvalidArgument,
			"currently only raw codec (%d) and dag-pb (%d) are accepted but got %d",
			cid.Raw,
			cid.DagProtobuf,
			codec,
		)
	}
	cidV1 := cid.NewCidV1(codec, requestCid.Hash())
	return cidV1.String(), nil
}

func (s *Server) doSearchCid(
	ctx context.Context, connectReq *connect.Request[pb.SearchCidRequest],
) (*connect.Response[pb.SearchCidResponse], error) {
	// TODO: add pagination later
	defaultVirtualServiceCount := int64(100)
	defaultInstanceCount := int64(100)
	req := connectReq.Msg
	cidV1, err := NormalizeCidToV1(req.GetCid())
	if err != nil {
		return nil, err
	}
	cidKey := fmt.Sprintf("cid:vsvc:%s", cidV1)
	it := s.redisClient.HScan(ctx, cidKey, 0, "", defaultVirtualServiceCount).Iterator()
	virtualServices := make([]*pb.VirtualService, 0, defaultVirtualServiceCount)
	for it.Next(ctx) {
		if err := it.Err(); err == redis.Nil {
			return nil, status.Error(
				codes.NotFound,
				"not found",
			)
		} else if err != nil {
			return nil, status.Errorf(
				codes.Internal,
				"failed to scan: %s",
				err.Error(),
			)
		}
		vHash := it.Val()
		if !it.Next(ctx) {
			break
		}
		// value is always 1 so ignored

		detailKey := fmt.Sprintf("vsvc:detail:%s", vHash)
		var vsvc pb.VirtualService
		if err := s.redisClient.Get(ctx, detailKey).Scan(&vsvc); err != nil {
			log.Printf("cannot find detail of virtual service %s", vHash)
			continue
		}
		virtualServices = append(virtualServices, &vsvc)
	}

	if len(virtualServices) == 0 {
		return nil, status.Errorf(
			codes.NotFound,
			"no virtual services with details found for cid %s",
			req.GetCid(),
		)
	}

	instances := make([]*AdWithScore, 0, defaultInstanceCount)
	for _, vsvc := range virtualServices {
		batchInstances, err := s.searchInstances(ctx, vsvc, defaultInstanceCount)
		if err != nil {
			continue
		}
		instances = append(instances, batchInstances...)
	}
	sort.Slice(instances, func(i, j int) bool {
		return instances[i].score < instances[j].score
	})
	ret := make([]*pb.ProviderAdvertise, 0, defaultInstanceCount)
	for i, instance := range instances {
		ret = append(ret, instance.ad)
		if i == int(defaultInstanceCount) {
			return connect.NewResponse(&pb.SearchCidResponse{
				VirtualServices:  virtualServices,
				StorageInstances: ret,
			}), nil
		}
	}
	return connect.NewResponse(&pb.SearchCidResponse{
		VirtualServices:  virtualServices,
		StorageInstances: ret,
	}), nil
}

func MustEncodeMultihash(c cid.Cid) string {
	mhBytes, err := base58.Decode(c.Hash().B58String())
	if err != nil {
		log.Fatalf("failed to decode base58: %s", err.Error())
	}
	mhString := base64.StdEncoding.EncodeToString(mhBytes)
	return mhString
}

func (s *Server) DelegatedRouting(
	ctx context.Context,
	connectReq *connect.Request[pb.DelegatedRoutingRequest],
) (*connect.Response[pb.DelegatedRoutingResponse], error) {
	searchCidResponse, err := s.doSearchCid(ctx, connect.NewRequest(&pb.SearchCidRequest{
		Cid: connectReq.Msg.GetCid(),
	}))
	if err != nil {
		return nil, err
	}
	var ret []*pb.Instance
	for _, instance := range searchCidResponse.Msg.GetStorageInstances() {
		ret = append(ret, instance.GetProviderInstance())
	}
	return connect.NewResponse(
		&pb.DelegatedRoutingResponse{
			Providers: ret,
		},
	), nil
}
