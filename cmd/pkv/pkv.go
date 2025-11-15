//go:generate protoc -I ../../pkg/proto -I ../../pkg/proto/googleapis --go_out=../../pkg/proto/gen/go --go_opt=paths=source_relative --go-grpc_out=../../pkg/proto/gen/go --go-grpc_opt=paths=source_relative --grpc-gateway_out=../../pkg/proto/gen/go --grpc-gateway_opt=paths=source_relative ../../pkg/proto/kvstore/kvstore.proto

package main

import (
	"fmt"
	"log"
	"net/http"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"buf.build/go/protovalidate"
	"connectrpc.com/connect"
	"connectrpc.com/grpcreflect"
	"github.com/atticplaygroup/pkv/internal/api"
	"github.com/atticplaygroup/pkv/pkg/middleware"
	"github.com/atticplaygroup/pkv/pkg/proto/gen/go/kvstore/v1/kvstoreconnect"
)

func main() {
	conf := api.LoadConfig(".env", ".")
	server, err := api.NewServer(&conf)
	if err != nil {
		log.Fatalf("cannot init server: %v", err)
	}
	defer server.GetRedisClient().Close()

	validator, err := protovalidate.New()
	if err != nil {
		log.Fatalf("failed to initialize validator: %s", err.Error())
	}
	sessionManager := middleware.NewRedisSessionManager(server.GetRedisClient())
	pricingManager := &middleware.PricingManager{}
	authManager := server.GetAuthManager()

	mux := http.NewServeMux()
	interceptors := make([]connect.Interceptor, 0)
	if !conf.DisableAuth {
		interceptors = append(
			interceptors,
			middleware.NewConnectUnarySessionInterceptor(sessionManager, pricingManager, authManager),
		)
	}
	interceptors = append(
		interceptors,
		middleware.NewConnectValidationInterceptor(validator),
	)

	path, handler := kvstoreconnect.NewKvStoreServiceHandler(
		server,
		connect.WithInterceptors(interceptors...),
	)
	mux.Handle(path, handler)
	reflector := grpcreflect.NewStaticReflector(
		kvstoreconnect.KvStoreServiceName,
	)
	rp1, rh1 := grpcreflect.NewHandlerV1(reflector)
	mux.Handle(rp1, rh1)
	rpa1, rha1 := grpcreflect.NewHandlerV1Alpha(reflector)
	mux.Handle(rpa1, rha1)
	c := api.GetCorsConfig()

	log.Printf("Server started at :%d\n", conf.GrpcPort)
	http.ListenAndServe(
		fmt.Sprintf("127.0.0.1:%d", conf.GrpcPort),
		h2c.NewHandler(c.Handler(mux), &http2.Server{}),
	)
}
