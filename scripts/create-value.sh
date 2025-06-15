#!/bin/bash

JWT_TOKEN=eyJhbGciOi...
VALUE=bar
VALUE_ENCODED="$(echo ${VALUE} | tr -d '\n' | tr -d ' ' | base64)"

# curl -X POST http://localhost:3000/v1/values:create \
# -H "grpc-metadata-x-prex-quota: Bearer ${JWT_TOKEN}" \
# -H "Content-Type: application/json" \
# -d '{"value": "'${VALUE_ENCODED}'", "ttl": "1000s"}'

grpcurl -plaintext \
  -H "x-prex-quota: Bearer ${JWT_TOKEN}" \
  -d '{"value": "'${VALUE_ENCODED}'"}' \
  localhost:50051 kvstore.pkv.proto.KvStore/CreateValue \
