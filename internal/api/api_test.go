package api_test

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/atticplaygroup/pkv/internal/api"
	"github.com/atticplaygroup/pkv/pkg/middleware"
	pb "github.com/atticplaygroup/pkv/pkg/proto/gen/go/kvstore/v1"
	"github.com/atticplaygroup/pkv/pkg/proto/gen/go/kvstore/v1/kvstoreconnect"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type MockJwtIssuer struct {
	privateKey ed25519.PrivateKey
	jwtSecret  []byte
}

var JwtSeed string
var JwtHS256Secret string
var GrpcClient *grpc.ClientConn
var sessionJwt string

const MOCK_SELF_IDENTIFIER = "did:example:pkv"

func getJwtClaimsEncoded(jwt string) string {
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		log.Fatalf("invalid jwt %s", jwt)
	}
	return parts[1]
}

func (m *MockJwtIssuer) parseJwtSub(jwtString string) string {
	token, _ := jwt.ParseWithClaims(jwtString, &middleware.CreateSessionJwtClaims{}, func(t *jwt.Token) (interface{}, error) {
		return "wrong salt", nil
	})
	if subject, err := token.Claims.GetSubject(); err != nil {
		panic(err)
	} else {
		return subject
	}
}

func NewMockJwtIssuer() *MockJwtIssuer {
	jwtSeedBytes := []byte{
		17, 17, 17, 17, 17, 17, 17, 17, 17, 17, 17, 17, 17, 17, 17, 17, 17, 17, 17, 17, 17, 17, 17, 17, 17, 17, 17, 17, 17, 17, 17, 17,
	}
	jwtSecret, err := base64.StdEncoding.DecodeString(JwtHS256Secret)
	if err != nil {
		log.Fatalf("config: failed to parse secret: %v", err)
	}
	ret := MockJwtIssuer{
		privateKey: ed25519.NewKeyFromSeed(jwtSeedBytes),
		jwtSecret:  jwtSecret,
	}
	return &ret
}

func (i *MockJwtIssuer) IssueQuotaToken(jti string) string {
	claims := middleware.CreateSessionJwtClaims{
		Quantity: 22222222,
		SessionJwtClaims: &middleware.SessionJwtClaims{
			RegisteredClaims: &jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(40 * time.Hour)),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				NotBefore: jwt.NewNumericDate(time.Now()),
				Issuer:    "did:key:z6MktULudTtAsAhRegYPiZ6631RV3viv12qd4GQF8z1xB22S",
				// Subject:   "b83dd883-7d0b-452f-88be-ea5bb4cb6061",
				ID:       jti,
				Audience: jwt.ClaimStrings{MOCK_SELF_IDENTIFIER},
			},
			Usage: pb.JwtUsage_JWT_USAGE_CREATE_SESSION,
		},
	}

	if claims.ID == "" {
		log.Fatalf("claim jti is null: %v", claims)
	}

	token := jwt.NewWithClaims(&jwt.SigningMethodEd25519{}, claims)
	jwt, err := token.SignedString(i.privateKey)
	if err != nil {
		log.Fatalf("failed to sign jwt: %v", err)
	}
	return jwt
}

var _ = Describe("Store, fetch and prolong data", Label("kvstore"), Ordered, func() {
	issuer := NewMockJwtIssuer()
	var resourceName string
	client := kvstoreconnect.NewKvStoreServiceClient(http.DefaultClient, "http://127.0.0.1:50051")
	ctx := context.Background()

	jti1 := uuid.NewString()

	cid1 := api.HashRawBytes([]byte("hello"))
	cid2 := api.HashRawBytes([]byte("world"))
	cid3 := api.HashRawBytes([]byte{
		0xdd, 0x79, 0xb5, 0x2c, 0x74, 0x3c, 0x1c, 0x9e, 0x83, 0xd0, 0x38, 0x20,
		0xcf, 0x27, 0xf4, 0x0d,
	})
	cids := []string{cid1, cid2, cid3}
	fmt.Printf("cids: %v\n", cids)
	root, err := api.CalculateMerkleRoot(cids)
	fmt.Printf("root: %s\n", root)
	ad := &pb.ProviderAdvertise{
		ProviderInstance: &pb.Instance{
			Did: "did:example:pkv2",
			// some random peer id for testing
			PeerId: "12D3KooWKiX28sD5zRiHgdwuNAgikjrHzKE7KaeWeN55DAUKMJnz",
			Multiaddrs: []string{
				"/dns4/pkv/tcp/8080/http",
			},
		},
		VirtualService: &pb.VirtualService{
			BehaviorLink: api.ServeAllBehavior,
			VariantLink: &pb.GlobalLink{
				Name:       root,
				Maintainer: "did:example:proposer",
				Version:    "v0.1.0",
			},
		},
		Cids:            cids,
		Price:           100,
		CoinType:        pb.CoinType_COIN_TYPE_SUI,
		CoinEnvironment: pb.CoinEnvironment_COIN_ENVIRONMENT_LOCALNET,
		Exchanges: []*pb.Instance{
			{
				Did:    "did:key:z6MktULudTtAsAhRegYPiZ6631RV3viv12qd4GQF8z1xB22S",
				PeerId: "12D3KooWQqiLXfqSPD36kno92S4xGFsEQ95j9umPgWtHc5v8iQhY",
				Multiaddrs: []string{
					"/dns4/prex/tcp/3000/http",
				},
			},
		},
		ExpireTime: timestamppb.New(time.Now().AddDate(0, 0, 7)),
		UpdateTime: timestamppb.New(time.Now().Add(-1 * time.Minute)),
		Signature:  "todo",
	}

	When("user creates new value without valid token", func() {
		It("should deny the request", func() {
			req := pb.CreateValueRequest{
				Codec: pb.CreateValueRequest_CODEC_RAW,
				Value: []byte("foo"),
				Ttl:   durationpb.New(1000 * time.Second),
			}
			_, err := client.CreateValue(ctx, connect.NewRequest(&req))
			Expect(err).To(Not(BeNil()))
		})
	})

	When("user login with valid token", func() {
		It("should succeed", func() {
			req := pb.CreateSessionRequest{
				Jwt: issuer.IssueQuotaToken(jti1),
			}
			resp, err := client.CreateSession(ctx, connect.NewRequest(&req))
			Expect(err).To(BeNil())
			Expect(len(resp.Msg.GetJwt())).To(Not(Equal("")))
			sessionJwt = resp.Msg.GetJwt()
			// Multiple request to login with the same jwt will map to the same session
			resp2, err := client.CreateSession(ctx, connect.NewRequest(&req))
			Expect(err).To(BeNil())
			Expect(issuer.parseJwtSub(resp2.Msg.GetJwt())).To(Equal(issuer.parseJwtSub(resp.Msg.GetJwt())))
		})
	})

	When("user creates new value with valid token", func() {
		It("should success", func() {
			req := pb.CreateValueRequest{
				Codec: pb.CreateValueRequest_CODEC_RAW,
				Value: []byte("foo"),
				Ttl:   durationpb.New(1000 * time.Second),
			}
			connectReq := connect.NewRequest(&req)
			connectReq.Header().Set(
				"authorization", "bearer "+sessionJwt,
			)
			resp, err := client.CreateValue(ctx, connectReq)
			Expect(err).To(BeNil())
			Expect(resp.Msg.GetTtl().AsDuration().Seconds()).To(Equal(1000.0))
			resourceName = resp.Msg.GetName()

			pbEncoded := "EisKIhIg8qaxa9MPTk+A02vTI5LGoNv8NODhF3EqUO5uuXXXDxcSABiu1OAVEisKIhIgpVQ3cvFr/ZOglfEBSf3ZHtXhhoJlswnWVK1PfPpXwyISABiu1OAVEisKIhIgv6B1FQC+ZCkS5I5Xgftx/1hri4OWSEdfkR8UXPx+5lESABiu1OAVEisKIhIg6WtJUWPf/PnJ8M2NJIB2D/YKiaYgHeIRxrqvHVtGOrwSABjNxsYOChsIAhjtjeZPIICA4BUggIDgFSCAgOAVIO2Nxg4="
			pbBytes, err := base64.StdEncoding.DecodeString(pbEncoded)
			Expect(err).To(BeNil())
			req = pb.CreateValueRequest{
				Codec: pb.CreateValueRequest_CODEC_DAG_PB,
				Value: pbBytes,
				Ttl:   durationpb.New(1000 * time.Second),
			}
			resp, err = client.CreateValue(ctx, connectReq)
			Expect(err).To(BeNil())
			Expect(resp.Msg.GetName()).To(Equal("values/bafybeigagd5nmnn2iys2f3doro7ydrevyr2mzarwidgadawmamiteydbzi"))
			Expect(resp.Msg.GetTtl().AsDuration().Seconds()).To(Equal(1000.0))
		})
	})

	// TODO: test with another sessionJwt should success because it is public
	When("user get the value", func() {
		It("should success", func() {
			req := pb.GetValueRequest{
				Name: resourceName,
			}
			connectReq := connect.NewRequest(&req)
			connectReq.Header().Set(
				"authorization", "bearer "+sessionJwt,
			)
			resp, err := client.GetValue(ctx, connectReq)
			Expect(err).To(BeNil())
			Expect(resp.Msg.GetValue()).To(Equal([]byte("foo")))
		})
	})

	When("user prolong ttl with invalid token", func() {
		It("should fail", func() {
			req := pb.ProlongValueRequest{
				Name: resourceName,
			}
			connectReq := connect.NewRequest(&req)
			connectReq.Header().Set(
				"authorization", "bearer "+"somethingInvalid",
			)
			_, err := client.ProlongValue(ctx, connectReq)
			Expect(err).To(Not(BeNil()))
		})
	})

	When("user prolong ttl with valid token", func() {
		It("should success", func() {
			req := pb.ProlongValueRequest{
				Name:    resourceName,
				Ttl:     durationpb.New(1000 * time.Second),
				MaxSize: 222,
			}
			connectReq := connect.NewRequest(&req)
			connectReq.Header().Set(
				"authorization", "bearer "+sessionJwt,
			)
			resp, err := client.ProlongValue(ctx, connectReq)
			Expect(err).To(BeNil())
			Expect(resp.Msg.GetTtl().AsDuration().Seconds()).To(BeNumerically("~", 2000.0, 10.0))
		})
	})

	When("register storage instance", func() {
		It("should success", func() {
			connectReq := connect.NewRequest(&pb.RegisterInstanceRequest{
				Advertisement: ad,
			})
			connectReq.Header().Add("authorization", "bearer "+sessionJwt)
			_, err = client.RegisterInstance(ctx, connectReq)
			Expect(err).To(BeNil())
		})
	})

	When("search cid", func() {
		It("should return the expected ad", func() {
			connectReq := connect.NewRequest(&pb.SearchCidRequest{
				Cid: cid1,
			})
			connectReq.Header().Add("authorization", "bearer "+sessionJwt)
			resp, err := client.SearchCid(ctx, connectReq)
			Expect(err).To(BeNil())
			vsvcs := resp.Msg.GetVirtualServices()
			Expect(len(vsvcs)).To(BeNumerically(">", 0))
			Expect(vsvcs[0].GetVariantLink().GetName()).To(Equal(root))
			instances := resp.Msg.GetStorageInstances()
			Expect(instances).To(HaveLen(1))
			Expect(instances[0].GetProviderInstance().GetDid()).To(Equal("did:example:pkv2"))
		})
	})

	When("search instances by virtual services", func() {
		It("should succeed", func() {
			connectReq := connect.NewRequest(&pb.SearchInstanceRequest{
				VirtualService: ad.GetVirtualService(),
			})
			connectReq.Header().Add("authorization", "bearer "+sessionJwt)
			resp, err := client.SearchInstance(ctx, connectReq)
			Expect(err).To(BeNil())
			Expect(api.GlobalLinkEqual(
				resp.Msg.GetVirtualService().GetBehaviorLink(),
				ad.GetVirtualService().GetBehaviorLink(),
			)).To(BeTrue())
			Expect(api.GlobalLinkEqual(
				resp.Msg.GetVirtualService().GetVariantLink(),
				ad.GetVirtualService().GetVariantLink(),
			)).To(BeTrue())
			instances := resp.Msg.GetInstancePriceInfo()
			Expect(instances).To(HaveLen(1))
			Expect(instances[0].GetProviderInstance().GetDid()).To(Equal("did:example:pkv2"))
		})
	})

	When("delegated routing", func() {
		It("should succeed", func() {
			connectReq := connect.NewRequest(&pb.DelegatedRoutingRequest{
				Cid: cid1,
			})
			connectReq.Header().Add("authorization", "bearer "+sessionJwt)
			resp, err := client.DelegatedRouting(ctx, connectReq)
			Expect(err).To(BeNil())
			mhr := resp.Msg.GetProviders()
			Expect(mhr).To(HaveLen(1))
		})
	})
})
