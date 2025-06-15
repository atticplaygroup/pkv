package middleware

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	durationpb "google.golang.org/protobuf/types/known/durationpb"
)

type QuotaTokenClaims struct {
	MaxSize int64         `json:"max_size"`
	Ttl     time.Duration `json:"ttl"`
	*jwt.RegisteredClaims
}

func ParseQuotaToken(ctx context.Context, jwtSecret ed25519.PublicKey) (*QuotaTokenClaims, error) {
	rawQuotaClaims, err := ParseHeaderJwt(ctx, &QuotaTokenClaims{}, jwtSecret, headerQuota, true)
	if err != nil {
		return nil, err
	}
	quotaClaims, ok := rawQuotaClaims.(*QuotaTokenClaims)
	if !ok {
		return nil, status.Errorf(
			codes.Unauthenticated,
			"failed to parse quota token claims",
		)
	}
	tokenId := quotaClaims.ID
	if tokenId == "" {
		return nil, status.Error(codes.Internal, "jti should not be empty")
	}

	return quotaClaims, nil
}

type RequestWithTtl interface {
	GetTtl() *durationpb.Duration
}

type RequestOrResponseWithValue interface {
	GetValue() []byte
}

type RequestOrResponseWithMaxSize interface {
	GetMaxSize() int64
}

func checkMaxSize(quotaClaims *QuotaTokenClaims, rawReq any) error {
	if req, ok := rawReq.(RequestOrResponseWithMaxSize); ok {
		if req == nil {
			return nil
		}
		if quotaClaims.MaxSize < 0 {
			return fmt.Errorf("max_size cannot be negative")
		}
		if req.GetMaxSize() != quotaClaims.MaxSize {
			return fmt.Errorf("max_size not matching quota token")
		}
	}
	return nil
}

func checkValueSize(quotaClaims *QuotaTokenClaims, rawReq any) error {
	if req, ok := rawReq.(RequestOrResponseWithValue); ok {
		if req == nil {
			return nil
		}
		if quotaClaims.MaxSize < 0 {
			return fmt.Errorf("max_size cannot be negative")
		}
		if int64(len(req.GetValue())) > quotaClaims.MaxSize {
			return fmt.Errorf("value length exceeds quota")
		}
	}
	return nil
}

func checkTtl(quotaClaims *QuotaTokenClaims, rawReq any) error {
	if req, ok := rawReq.(RequestWithTtl); ok {
		if req == nil {
			return nil
		}
		if quotaClaims.Ttl < 0 {
			return fmt.Errorf("ttl cannot be negative")
		}
		if req.GetTtl().AsDuration() == quotaClaims.Ttl {
			return fmt.Errorf("ttl mismatch with quota token")
		}
	}
	return nil
}

func QuotaTokenValidityMiddleware(
	jwtSecret ed25519.PublicKey,
) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		quotaClaims, err := ParseQuotaToken(ctx, jwtSecret)
		if err != nil {
			return nil, status.Errorf(
				codes.Unauthenticated,
				"quota token not found or corrupted: %v",
				err,
			)
		}
		if err = checkMaxSize(quotaClaims, req); err != nil {
			return nil, status.Error(
				codes.PermissionDenied,
				err.Error(),
			)
		}
		if err = checkValueSize(quotaClaims, req); err != nil {
			return nil, status.Error(
				codes.PermissionDenied,
				err.Error(),
			)
		}
		if err = checkTtl(quotaClaims, req); err != nil {
			return nil, status.Error(
				codes.PermissionDenied,
				err.Error(),
			)
		}
		resp, err := handler(ctx, req)
		if err != nil {
			return nil, err
		}
		if err = checkValueSize(quotaClaims, resp); err != nil {
			return nil, err
		}
		return resp, nil
	}
}

type QuotaTokenNullifer struct {
	Rdb *redis.Client
}

func QuotaTokenSelector(ctx context.Context, callMeta interceptors.CallMeta) bool {
	return true
}

func (r *QuotaTokenNullifer) QuotaTokenNullifyMiddleware(
	jwtSecret ed25519.PublicKey,
) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		quotaClaims, err := ParseQuotaToken(ctx, jwtSecret)
		if err != nil {
			return nil, status.Errorf(
				codes.Unauthenticated,
				"quota token not found or corrupted",
			)
		}
		quotaKey := fmt.Sprintf("quota/%s", quotaClaims.ID)
		quotaValue := "1"
		expireAt, err := quotaClaims.GetExpirationTime()
		if err != nil {
			return nil, status.Errorf(
				codes.InvalidArgument,
				"exp not found in jwt",
			)
		}
		ttl := time.Until(expireAt.Time)
		if err := r.Rdb.SetNX(ctx, quotaKey, quotaValue, ttl).Err(); err != nil {
			return nil, status.Error(codes.Internal, "token has been used or connection error")
		}
		return handler(ctx, req)
	}
}
