package api

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	pb "github.com/atticplaygroup/pkv/pkg/proto/gen/go/kvstore/v1"
	"github.com/ipfs/go-cid"
	"github.com/mr-tron/base58"
	"github.com/multiformats/go-multihash"
	"github.com/redis/go-redis/v9"
	"github.com/wealdtech/go-merkletree/v2"
	"github.com/wealdtech/go-merkletree/v2/keccak256"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const (
	SERVE_ALL_BEHAVIOR_NAME       = "serve_all"
	SERVE_ALL_BEHAVIOR_MAINTAINER = "did:example:foo"
	SERVE_ALL_BEHAVIOR_VERSION    = "v0.1.0"
	DEFAULT_TTL                   = 7 * 24 * time.Hour

	MAX_CID_SIZE = 1 << 16
)

var ServeAllBehavior = &pb.GlobalLink{
	Name:       SERVE_ALL_BEHAVIOR_NAME,
	Maintainer: SERVE_ALL_BEHAVIOR_MAINTAINER,
	Version:    SERVE_ALL_BEHAVIOR_VERSION,
}

func CalculateMerkleRoot(cidStrings []string) (string, error) {
	cids := make([][]byte, 0, len(cidStrings))
	for _, cidString := range cidStrings {
		cid, err := cid.Decode(cidString)
		if err != nil {
			return "", status.Errorf(
				codes.InvalidArgument,
				"failed to parse cid %s: %v",
				cidString,
				err,
			)
		}
		cids = append(cids, cid.Bytes())
	}
	mt, err := merkletree.NewTree(
		merkletree.WithData(cids),
		merkletree.WithHashType(&keccak256.Keccak256{}),
		merkletree.WithSorted(false),
	)
	if err != nil {
		return "", status.Errorf(
			codes.InvalidArgument,
			"failed to create merkle tree: %v",
			err,
		)
	}
	mh, err := multihash.Encode(mt.Root(), multihash.KECCAK_256)
	if err != nil {
		return "", status.Errorf(
			codes.Internal,
			"failed to encode merkle root: %v",
			err,
		)
	}
	return cid.NewCidV1(cid.Raw, mh).String(), nil
}

func HashMessage[T proto.Message](m T) (string, error) {
	message, err := proto.Marshal(m)
	if err != nil {
		return "", err
	}
	resultBytes := sha256.Sum256(message)
	result := base58.Encode(resultBytes[:])
	return result, nil
}

func GlobalLinkEqual(a *pb.GlobalLink, b *pb.GlobalLink) (bool, error) {
	aHash, err := HashMessage(a)
	if err != nil {
		return false, err
	}
	bHash, err := HashMessage(b)
	if err != nil {
		return false, err
	}
	return strings.EqualFold(aHash, bHash), nil
}

type MerkleTreeFileServing interface {
	IsBehaviorMatching(behaviorLink *pb.GlobalLink) (bool, error)
	EnsureAdvertisementValid(advertisement *pb.ProviderAdvertise) error
	Register(ctx context.Context, advertisement *pb.ProviderAdvertise) error
}

type ServeAllFileServing struct {
	redisClient *redis.Client
}

func NewServeAllFileServing(redisClient *redis.Client) MerkleTreeFileServing {
	return &ServeAllFileServing{
		redisClient: redisClient,
	}
}

func (m *ServeAllFileServing) IsBehaviorMatching(behaviorLink *pb.GlobalLink) (bool, error) {
	return GlobalLinkEqual(behaviorLink, ServeAllBehavior)
}

func (m *ServeAllFileServing) EnsureAdvertisementValid(advertisement *pb.ProviderAdvertise) error {
	// TODO: check variantLink signature when the reputation of maintainer matters
	variant := advertisement.GetVirtualService().GetVariantLink()
	expectedRoot, err := CalculateMerkleRoot(advertisement.GetCids())
	if err != nil {
		return err
	}
	if expectedRoot != variant.GetName() {
		return fmt.Errorf("merkle tree root not matching, expected %s got %s", expectedRoot, variant.GetName())
	}
	return nil
}

func (m *ServeAllFileServing) registerVirtualService(
	ctx context.Context,
	advertisement *pb.ProviderAdvertise,
) error {
	virtualServiceHash, err := HashMessage(advertisement.GetVirtualService())
	if err != nil {
		return err
	}
	cidMap := make(map[string]interface{}, len(advertisement.GetCids()))
	for _, cid := range advertisement.GetCids() {
		cidMap[cid] = 1
	}
	cidKey := fmt.Sprintf("vsvc:cid:%s", virtualServiceHash)
	if _, err := m.redisClient.HSet(ctx, cidKey, cidMap).Result(); err != nil {
		return err
	}
	if err = m.redisClient.Expire(ctx, cidKey, DEFAULT_TTL).Err(); err != nil {
		return err
	}

	detailKey := fmt.Sprintf("vsvc:detail:%s", virtualServiceHash)
	if err = m.redisClient.Set(ctx, detailKey, advertisement.GetVirtualService(), DEFAULT_TTL).Err(); err != nil {
		return err
	}

	// A naive score sorting provider instances by the time indexed, to be optimized later
	score := time.Now().UnixMilli()
	scoreKey := fmt.Sprintf("vsvc:instance:%s", virtualServiceHash)
	if err = m.redisClient.ZAddGT(ctx, scoreKey, redis.Z{
		Score:  float64(score),
		Member: advertisement.GetProviderInstance().GetDid(),
	}).Err(); err != nil {
		return err
	}
	// Refresh TTL. It will expire earliest at this time, in case no other instances updates this virtual service.
	if err = m.redisClient.Expire(ctx, scoreKey, DEFAULT_TTL).Err(); err != nil {
		return err
	}

	instanceKey := fmt.Sprintf("instance:%s", advertisement.GetProviderInstance().GetDid())
	if err = m.redisClient.Set(ctx, instanceKey, advertisement, DEFAULT_TTL).Err(); err != nil {
		return err
	}

	return nil
}

func (m *ServeAllFileServing) registerCid(
	ctx context.Context,
	advertisement *pb.ProviderAdvertise,
) error {
	virtualServiceHash, err := HashMessage(advertisement.GetVirtualService())
	if err != nil {
		return err
	}
	for _, cid := range advertisement.GetCids() {
		cidKey := fmt.Sprintf("cid:vsvc:%s", cid)
		if _, err := m.redisClient.HSet(ctx, cidKey, map[string]interface{}{virtualServiceHash: 1}).Result(); err != nil {
			return err
		}
		if err = m.redisClient.HExpire(ctx, cidKey, DEFAULT_TTL, virtualServiceHash).Err(); err != nil {
			return err
		}
	}
	return nil
}

func (m *ServeAllFileServing) Register(
	ctx context.Context, advertisement *pb.ProviderAdvertise,
) error {
	if err := m.registerVirtualService(ctx, advertisement); err != nil {
		return err
	}
	if err := m.registerCid(ctx, advertisement); err != nil {
		return err
	}
	return nil
}
