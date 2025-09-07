#!/bin/bash

set -eu

export SIGNING_SEED="$(cat /dev/urandom | head -c 32 | base64)"

[ -d ./bin ] || mkdir bin
go build -o bin cmd/pkv/pkv.go
./bin/pkv &
SERVER_PID=$!
sleep 1
ginkgo internal/*
kill ${SERVER_PID}
