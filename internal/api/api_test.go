package api_test

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"log"
	"time"

	"github.com/atticplaygroup/pkv/pkg/middleware"
	pb "github.com/atticplaygroup/pkv/pkg/proto/gen/go/kvstore"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
)

type MockJwtIssuer struct {
	privateKey ed25519.PrivateKey
	jwtSecret  []byte
}

var JwtSeed string
var JwtHS256Secret string
var GrpcClient *grpc.ClientConn

func NewMockJwtIssuer() *MockJwtIssuer {
	jwtSeedBytes, err := base64.StdEncoding.DecodeString(JwtSeed)
	if err != nil {
		log.Fatalf("config: failed to parse secret: %v", err)
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

func (i *MockJwtIssuer) IssueAuthToken(jti string) string {
	claims := jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(40 * time.Second)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		NotBefore: jwt.NewNumericDate(time.Now()),
		Issuer:    "test",
		Subject:   "b83dd883-7d0b-452f-88be-ea5bb4cb6061",
		ID:        jti,
		Audience:  []string{"somebody_else"},
	}
	if claims.ID == "" {
		log.Fatalf("claim jti is null: %v", claims)
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	jwt, err := token.SignedString(i.jwtSecret)
	if err != nil {
		log.Fatalf("failed to sign jwt: %v", err)
	}
	return jwt
}

func (i *MockJwtIssuer) IssueQuotaToken(jti string) string {
	claims := middleware.QuotaTokenClaims{
		Ttl:     1000,
		MaxSize: 222,
		RegisteredClaims: &jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(40 * time.Second)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "test",
			Subject:   "b83dd883-7d0b-452f-88be-ea5bb4cb6061",
			ID:        jti,
			Audience:  []string{"somebody_else"},
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
	client := pb.NewKvStoreClient(GrpcClient)
	ctx := context.Background()

	jti1 := uuid.NewString()
	jti2 := uuid.NewString()
	jti3 := uuid.NewString()

	When("user creates new value without valid token", func() {
		It("should deny the request", func() {
			req := pb.CreateValueRequest{
				Value:  []byte("foo"),
				Parent: "accounts/b83dd883-7d0b-452f-88be-ea5bb4cb6061",
				Ttl:    durationpb.New(1000 * time.Second),
			}
			_, err := client.CreateValue(ctx, &req)
			Expect(err).To(Not(BeNil()))
			grpcErr, ok := status.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(grpcErr.Code()).To(Equal(codes.Unauthenticated))
		})
	})

	When("user creates new value with valid token", func() {
		It("should success", func() {
			req := pb.CreateValueRequest{
				Value:  []byte("foo"),
				Parent: "accounts/b83dd883-7d0b-452f-88be-ea5bb4cb6061",
				Ttl:    durationpb.New(1000 * time.Second),
			}
			md := metadata.Pairs("x-prex-quota", "Bearer "+issuer.IssueQuotaToken(jti1))
			resp, err := client.CreateValue(metadata.NewOutgoingContext(ctx, md), &req)
			Expect(err).To(BeNil())
			Expect(resp.GetTtl().AsDuration().Seconds()).To(Equal(1000.0))
			resourceName = resp.GetName()
		})
	})

	When("user get the value", func() {
		It("should success", func() {
			req := pb.GetValueRequest{
				Name: resourceName,
			}
			md := metadata.Pairs(
				"x-prex-quota", "Bearer "+issuer.IssueQuotaToken(jti1),
				"authorization", "Bearer "+issuer.IssueAuthToken(jti3),
			)
			resp, err := client.GetValue(metadata.NewOutgoingContext(ctx, md), &req)
			Expect(err).To(BeNil())
			Expect(resp.GetValue()).To(Equal([]byte("foo")))
		})
	})

	When("user prolong ttl with invalid token", func() {
		It("should fail", func() {
			req := pb.ProlongValueRequest{
				Name: resourceName,
			}
			md := metadata.Pairs("x-prex-quota", "Bearer "+issuer.IssueQuotaToken(jti1))
			_, err := client.ProlongValue(metadata.NewOutgoingContext(ctx, md), &req)
			Expect(err).To(Not(BeNil()))
			grpcErr, ok := status.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(grpcErr.Code()).To(Equal(codes.PermissionDenied))
		})
	})

	When("user prolong ttl with valid token", func() {
		It("should success", func() {
			req := pb.ProlongValueRequest{
				Name:    resourceName,
				Ttl:     durationpb.New(1000 * time.Second),
				MaxSize: 222,
			}
			md := metadata.Pairs("x-prex-quota", "Bearer "+issuer.IssueQuotaToken(jti2))
			resp, err := client.ProlongValue(metadata.NewOutgoingContext(ctx, md), &req)
			Expect(err).To(BeNil())
			Expect(resp.GetTtl().AsDuration().Seconds()).To(BeNumerically("~", 2000.0, 10.0))
		})
	})
})
