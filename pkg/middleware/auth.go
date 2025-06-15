package middleware

import (
	"context"
	"fmt"

	pb "github.com/atticplaygroup/pkv/pkg/proto/gen/go/kvstore"
	"github.com/golang-jwt/jwt/v5"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type AuthClaims struct {
	AccountId string
}

func ParseAuthToken(ctx context.Context, jwtSecret []byte, withValidation bool) (*AuthClaims, error) {
	rawAuthclaims, err := ParseHeaderJwt(ctx, &jwt.RegisteredClaims{}, jwtSecret, headerAuthorize, withValidation)
	if err != nil {
		return nil, err
	}
	authClaims, ok := rawAuthclaims.(*jwt.RegisteredClaims)
	if !ok {
		return nil, status.Errorf(
			codes.Unauthenticated,
			"failed to parse auth claims: %+v",
			rawAuthclaims,
		)
	}
	subject, err := authClaims.GetSubject()
	if err != nil {
		return nil, status.Errorf(
			codes.Unauthenticated,
			"cannot find subject in jwt",
		)
	}
	return &AuthClaims{AccountId: subject}, nil
}

type Resource interface {
	GetName() string
}

type ResourceParent interface {
	GetParent() string
}

func checkResourceParentField(req any, filedName string, value string) error {
	if resourceRequest, ok := req.(ResourceParent); ok {
		if resourceRequest == nil {
			return nil
		}
		if resourceRequest.GetParent() == "" {
			return fmt.Errorf("got empty parent")
		}
		if idSegments, err := ParseResourceName(
			resourceRequest.GetParent(), []string{filedName}); err != nil {
			return nil
		} else {
			if len(idSegments) != 1 {
				return fmt.Errorf("idSegment parse failed: %v", idSegments)
			}
			if idSegments[0] != value {
				return fmt.Errorf("%s in resource parent mismatch: %s vs %s", filedName, idSegments[0], value)
			}
			return nil
		}
	}
	return nil
}

func checkResourceField(req any, filedName string, value string) error {
	if resourceRequest, ok := req.(Resource); ok {
		if resourceRequest == nil {
			return nil
		}
		if resourceRequest.GetName() == "" {
			return fmt.Errorf("got empty name")
		}
		if idSegments, err := ParseResourceName(
			resourceRequest.GetName(), []string{filedName}); err != nil {
			return nil
		} else {
			if len(idSegments) != 1 {
				return fmt.Errorf("idSegment parse failed: %v", idSegments)
			}
			if idSegments[0] != value {
				return fmt.Errorf("%s in resource parent mismatch: %s vs %s", filedName, idSegments[0], value)
			}
			return nil
		}
	}
	return nil
}

func AuthTokenMiddleware(jwtSecret []byte) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		authClaims, err := ParseAuthToken(ctx, jwtSecret, true)
		if err != nil || authClaims.AccountId == "" {
			return nil, status.Errorf(
				codes.Unauthenticated,
				"account id is invalid: %v",
				err,
			)
		}
		return handler(context.WithValue(ctx, KeyAuthClaims, authClaims), req)
	}
}

func ResourceAccountFieldCheckerMiddleware() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		authClaims := ctx.Value(KeyAuthClaims).(*AuthClaims)
		if authClaims.AccountId == "" {
			return nil, status.Error(
				codes.Unauthenticated,
				"account id is invalid",
			)
		}
		if err := checkResourceField(req, "accounts", authClaims.AccountId); err != nil {
			return nil, status.Errorf(
				codes.PermissionDenied,
				"account id %s not matching that in resource name: %v",
				authClaims.AccountId,
				err,
			)
		}
		if err := checkResourceParentField(req, "accounts", authClaims.AccountId); err != nil {
			return nil, status.Errorf(
				codes.PermissionDenied,
				"account id %s not matching that in resource parent: %v",
				authClaims.AccountId,
				err,
			)
		}
		return handler(ctx, req)
	}
}

func AuthMiddlewareSelector(ctx context.Context, callMeta interceptors.CallMeta) bool {
	for _, authRequired := range []string{
		pb.KvStore_GetValue_FullMethodName,
		pb.KvStore_GetStreamValue_FullMethodName,
	} {
		if callMeta.FullMethod() == authRequired {
			return true
		}
	}
	return false
}
