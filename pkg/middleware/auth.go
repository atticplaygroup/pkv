package middleware

import (
	"crypto/ed25519"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type IAuthManager interface {
	VerifyAndParseJwt(jwtString string, claims jwt.Claims, isSelf bool) (jwt.Claims, error)
}

type StaticAuthManager struct {
	jwtSecret      []byte
	trustedIssuers map[string]ed25519.PublicKey
	selfIdentifier string
}

func NewStaticAuthManager(jwtSecret []byte, trustedIssuers map[string]ed25519.PublicKey, selfIdentifier string) *StaticAuthManager {
	return &StaticAuthManager{
		jwtSecret:      jwtSecret,
		trustedIssuers: trustedIssuers,
		selfIdentifier: selfIdentifier,
	}
}

func (a *StaticAuthManager) selfKeyFunc(token *jwt.Token) (any, error) {
	if method, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
		return nil, status.Errorf(
			codes.Unauthenticated,
			"invalid signing method %s",
			token.Method.Alg(),
		)
	} else if method.Name != "HS256" {
		return nil, status.Errorf(
			codes.Unauthenticated,
			"invalid signing method %s",
			token.Method.Alg(),
		)
	}
	return a.jwtSecret, nil
}

func (a *StaticAuthManager) exchangeKeyFunc(token *jwt.Token) (any, error) {
	if _, ok := token.Method.(*jwt.SigningMethodEd25519); !ok {
		return nil, status.Errorf(
			codes.Unauthenticated,
			"invalid signing method %s",
			token.Method.Alg(),
		)
	}
	issuer, err := token.Claims.GetIssuer()
	if err != nil {
		return nil, status.Errorf(
			codes.Unauthenticated,
			"failed to get issuer %s",
			err.Error(),
		)
	}
	publicKey, ok := a.trustedIssuers[issuer]
	if !ok {
		return nil, status.Errorf(
			codes.PermissionDenied,
			"unexpected issuer %s",
			issuer,
		)
	}
	return publicKey, nil
}

func (a *StaticAuthManager) VerifyAndParseJwt(jwtString string, claims jwt.Claims, isSelf bool) (jwt.Claims, error) {
	keyFunc := a.exchangeKeyFunc
	if isSelf {
		keyFunc = a.selfKeyFunc
	}
	token, err := jwt.ParseWithClaims(
		jwtString, claims, keyFunc,
	)
	if err != nil {
		return nil, status.Errorf(
			codes.PermissionDenied,
			"jwt validation failed: %s",
			err.Error(),
		)
	}
	return token.Claims, nil
}
