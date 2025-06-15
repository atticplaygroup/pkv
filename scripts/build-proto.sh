#!/bin/bash

[ -d ./pkg/proto/googleapis ] || git clone https://github.com/googleapis/googleapis.git ./pkg/proto/

mkdir -p ./pkg/proto/gen/go

protoc -I ./pkg/proto \
    -I ./pkg/proto/googleapis \
    --go_out=./pkg/proto/gen/go \
    --go_opt=paths=source_relative \
    --go-grpc_out=./pkg/proto/gen/go \
    --go-grpc_opt=paths=source_relative \
    --grpc-gateway_out=./pkg/proto/gen/go \
    --grpc-gateway_opt=paths=source_relative \
    --grpc-gateway_opt=generate_unbound_methods=true \
    --openapiv2_out=./pkg/proto/gen/openapi \
    --openapiv2_opt=use_go_templates=true \
    ./pkg/proto/kvstore/kvstore.proto
