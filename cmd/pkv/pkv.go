//go:generate protoc -I ../../pkg/proto -I ../../pkg/proto/googleapis --go_out=../../pkg/proto/gen/go --go_opt=paths=source_relative --go-grpc_out=../../pkg/proto/gen/go --go-grpc_opt=paths=source_relative --grpc-gateway_out=../../pkg/proto/gen/go --grpc-gateway_opt=paths=source_relative ../../pkg/proto/kvstore/kvstore.proto

package main

import (
	"context"
	"fmt"
	"log"
	"net"

	"github.com/atticplaygroup/pkv/internal/api"
	"github.com/atticplaygroup/pkv/pkg/middleware"
	"github.com/atticplaygroup/pkv/pkg/proto/gen/go/kvstore"
	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/selector"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	conf := api.LoadConfig(".env", ".")
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", conf.GrpcPort))
	if err != nil {
		log.Fatalf("listen failed: %v", err)
	}

	ctx := context.Background()

	server, err := api.NewServer(&conf)
	if err != nil {
		log.Fatalf("cannot init server: %v", err)
	}
	tokenNullifier := middleware.QuotaTokenNullifer{
		Rdb: server.GetRedisClient(),
	}
	s := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			selector.UnaryServerInterceptor(
				middleware.AuthTokenMiddleware(conf.JwtSecret),
				selector.MatchFunc(middleware.AuthMiddlewareSelector),
			),
			selector.UnaryServerInterceptor(
				middleware.ResourceAccountFieldCheckerMiddleware(),
				selector.MatchFunc(middleware.AuthMiddlewareSelector),
			),
			selector.UnaryServerInterceptor(
				middleware.QuotaTokenValidityMiddleware(conf.QuotaAuthorityPublicKey),
				selector.MatchFunc(middleware.QuotaTokenSelector),
			),
			selector.UnaryServerInterceptor(
				tokenNullifier.QuotaTokenNullifyMiddleware(conf.QuotaAuthorityPublicKey),
				selector.MatchFunc(middleware.QuotaTokenSelector),
			),
		),
	)
	reflection.Register(s)
	kvstore.RegisterKvStoreServer(s, server)
	go func() {
		defer s.GracefulStop()
		<-ctx.Done()
	}()
	s.Serve(l)
}
