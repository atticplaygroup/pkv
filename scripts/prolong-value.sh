#!/bin/bash

set -eu

NAME="${1}"

JWT_TOKEN=eyJhbGciOiJ...

curl -X POST http://localhost:3000/v1/"${NAME}":prolong \
-H "Authorization: Bearer ${JWT_TOKEN}" \
-H "Content-Type: application/json" \
-d '{"name": "'"${NAME}"'"}'

# grpcurl -plaintext \
#   -H "Authorization: Bearer ${JWT_TOKEN}" \
#   -d '{"name": "'"${NAME}"'"}' \
#   localhost:50051 kvstore.pkv.proto.KvStore/ProlongValue \
