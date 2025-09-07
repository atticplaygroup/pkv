package api

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"github.com/atticplaygroup/pkv/pkg/middleware"
	pb "github.com/atticplaygroup/pkv/pkg/proto/gen/go/kvstore/v1"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) generateJwt(sessionId string, expireAt time.Time) (string, error) {
	claims := &middleware.SessionJwtClaims{
		RegisteredClaims: &jwt.RegisteredClaims{
			Issuer:    s.config.SelfIdentifier,
			Audience:  jwt.ClaimStrings{s.config.SelfIdentifier},
			ExpiresAt: jwt.NewNumericDate(expireAt),
			Subject:   sessionId,
			ID:        uuid.NewString(),
		},
		Usage: pb.JwtUsage_JWT_USAGE_MANAGE_SESSION,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token.Header["kid"] = s.config.TokenSigningKeyId
	jwt, err := token.SignedString(s.config.JwtSecret)
	if err != nil {
		return "", status.Errorf(
			codes.InvalidArgument,
			"failed to sign jwt: %v",
			err,
		)
	}
	return jwt, err
}

func (s *Server) CreateSession(
	ctx context.Context, req *connect.Request[pb.CreateSessionRequest],
) (*connect.Response[pb.CreateSessionResponse], error) {
	claims, err := s.authmanager.VerifyAndParseJwt(req.Msg.GetJwt(), &middleware.CreateSessionJwtClaims{}, false)
	if err != nil {
		return nil, err
	}
	createSessionClaims, ok := claims.(*middleware.CreateSessionJwtClaims)
	if !ok {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"failed to parse session jwt",
		)
	}
	if createSessionClaims.Usage != pb.JwtUsage_JWT_USAGE_CREATE_SESSION {
		return nil, status.Errorf(
			codes.PermissionDenied,
			"expected token usage %d but got %d",
			pb.JwtUsage_JWT_USAGE_CREATE_SESSION,
			createSessionClaims.Usage,
		)
	}
	if createSessionClaims.Quantity <= 0 {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"initial balance must be positive but go5 %d",
			createSessionClaims.Quantity,
		)
	}
	expireAt, err := createSessionClaims.GetExpirationTime()
	if err != nil || expireAt == nil || time.Until(expireAt.Time) < 0 {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"failed to parse expire time: %s",
			err.Error(),
		)
	}
	session, err := s.sessionManager.CreateSession(
		ctx, createSessionClaims.ID, createSessionClaims.Quantity, time.Until(expireAt.Time),
	)
	if err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"failed to create session: %s",
			err.Error(),
		)
	}
	jwt, err := s.generateJwt(session.GetSessionId(), expireAt.Time)
	if err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"failed to create jwt: %s",
			err.Error(),
		)
	}
	return connect.NewResponse(&pb.CreateSessionResponse{
		Session: session,
		Jwt:     jwt,
	}), nil
}
