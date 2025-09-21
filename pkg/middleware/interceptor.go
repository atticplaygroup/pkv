package middleware

import (
	"context"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"buf.build/go/protovalidate"
	"connectrpc.com/connect"

	pb "github.com/atticplaygroup/pkv/pkg/proto/gen/go/kvstore/v1"
	pbc "github.com/atticplaygroup/pkv/pkg/proto/gen/go/kvstore/v1/kvstoreconnect"
	"google.golang.org/protobuf/proto"
)

func NewConnectUnarySessionInterceptor(s ISessionManager, p IPricingManager, a IAuthManager) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			skipAuth := []string{
				pbc.KvStoreServiceCreateSessionProcedure,
				pbc.KvStoreServiceSearchCidProcedure,
				pbc.KvStoreServiceSearchInstanceProcedure,
				pbc.KvStoreServicePingProcedure,
				pbc.KvStoreServiceDelegatedRoutingProcedure,
			}
			for _, procedureName := range skipAuth {
				if req.Spec().Procedure == procedureName {
					return next(ctx, req)
				}
			}

			authString := req.Header().Get("Authorization")
			if len(authString) == 0 {
				authString = req.Header().Get("authorization")
			}
			pieces := strings.Split(authString, " ")
			if len(pieces) != 2 || !strings.EqualFold(pieces[0], "bearer") {
				return nil, status.Errorf(
					codes.InvalidArgument,
					"failed to parse bearer token",
				)
			}
			claims, err := a.VerifyAndParseJwt(pieces[1], &SessionJwtClaims{}, true)
			if err != nil {
				return nil, status.Errorf(
					codes.PermissionDenied,
					"failed to parse or verify token: %s",
					err.Error(),
				)
			}

			jwtClaims, ok := claims.(*SessionJwtClaims)
			if !ok {
				return nil, status.Errorf(
					codes.InvalidArgument,
					"failed to parse session jwt",
				)
			}
			if jwtClaims.Usage != pb.JwtUsage_JWT_USAGE_MANAGE_SESSION {
				return nil, status.Errorf(
					codes.PermissionDenied,
					"expected token usage %d but got %d",
					pb.JwtUsage_JWT_USAGE_MANAGE_SESSION,
					jwtClaims.Usage,
				)
			}
			price, err := p.GetPrice(req)
			if err != nil {
				return nil, status.Errorf(
					codes.Internal,
					"failed to get price: %s",
					err.Error(),
				)
			}
			session, err := s.DeductSessionBalance(ctx, jwtClaims.Subject, price)
			if err != nil {
				return nil, status.Errorf(
					codes.Internal,
					"failed to deduct: %s",
					err.Error(),
				)
			}
			return next(context.WithValue(ctx, KeySession, session), req)
		}
	}
}

func protoValidation(req any, v protovalidate.Validator) error {
	m, ok := req.(proto.Message)
	if !ok {
		return status.Error(
			codes.InvalidArgument,
			"failed to parse proto",
		)
	}
	if err := v.Validate(m); err != nil {
		return status.Errorf(
			codes.InvalidArgument,
			"failed to parse proto: %s",
			err.Error(),
		)
	}
	return nil
}

func NewConnectValidationInterceptor(v protovalidate.Validator) connect.UnaryInterceptorFunc {
	interceptor := func(next connect.UnaryFunc) connect.UnaryFunc {
		return connect.UnaryFunc(func(
			ctx context.Context,
			req connect.AnyRequest,
		) (connect.AnyResponse, error) {
			if err := protoValidation(req.Any(), v); err != nil {
				return nil, err
			}
			return next(ctx, req)
		})
	}
	return connect.UnaryInterceptorFunc(interceptor)
}
