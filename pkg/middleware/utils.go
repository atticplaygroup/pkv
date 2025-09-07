package middleware

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type CtxKeyType int

const (
	KeyAuthClaims CtxKeyType = iota
	KeySession
)

func parseResourceName(name string, prefixes []string) ([]string, error) {
	pattern := strings.Join(prefixes, `/([0-9a-zA-Z\-@\.]+)/`) + `/([0-9a-zA-Z\-@\.]+)`
	r := regexp.MustCompile(pattern)
	matches := r.FindStringSubmatch(name)

	if len(matches) == len(prefixes)+1 {
		return matches[1:], nil
	} else {
		return nil, fmt.Errorf("resource name %s not matching pattern %s", name, pattern)
	}
}

func ParseResourceName(name string, fields []string) ([]string, error) {
	idSegments, err := parseResourceName(name, fields)
	if err != nil || len(idSegments) != len(fields) {
		return nil, fmt.Errorf(
			"cannot parse resource name: %v",
			err,
		)
	}
	for i, id := range idSegments {
		if id == "" {
			return nil, fmt.Errorf(
				"field %s is empty",
				fields[i],
			)
		}
	}
	return idSegments, nil
}

// Adapted from grpcauth.AuthFromMD but allows other than authorization header
func TokenFromMD(ctx context.Context, headerField string) (string, error) {
	expectedScheme := "bearer"
	vals := metadata.ValueFromIncomingContext(ctx, headerField)
	if len(vals) == 0 {
		return "", status.Error(codes.Unauthenticated, "Request unauthenticated with "+expectedScheme)
	}
	scheme, token, found := strings.Cut(vals[0], " ")
	if !found {
		return "", status.Error(codes.Unauthenticated, "Bad authorization string")
	}
	if !strings.EqualFold(scheme, expectedScheme) {
		return "", status.Error(codes.Unauthenticated, "Request unauthenticated with "+expectedScheme)
	}
	return token, nil
}

func ParseHeaderJwt(
	ctx context.Context,
	claims jwt.Claims,
	// quotaAuthorityPublicKey ed25519.PublicKey,
	// jwtSecret []byte,
	witness any,
	headerField string,
	withValdidation bool,
) (interface{}, error) {
	tokenString, err := TokenFromMD(ctx, headerField)
	if err != nil {
		return nil, status.Errorf(
			codes.Unauthenticated,
			"invalid bearer token: %v",
			err,
		)
	}
	options := []jwt.ParserOption{}
	if !withValdidation {
		// TODO: add jwt.WithAudience and WithIssuer to check them
		options = append(options, jwt.WithoutClaimsValidation())
	}
	token, err := jwt.ParseWithClaims(
		tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodEd25519); ok {
				return witness, nil
			}
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); ok {
				return witness, nil
			}
			return nil, status.Errorf(
				codes.Unauthenticated,
				"invalid signing method %s",
				token.Method.Alg(),
			)
		},
		options...,
	)
	if err != nil || !token.Valid {
		return nil, status.Errorf(
			codes.Unauthenticated,
			"invalid token of %s: %v",
			headerField,
			err,
		)
	}
	return token.Claims, nil
}
