#!/bin/bash

set -eu

NAME="$1"
curl -X GET "http://localhost:3000/v1/${NAME}" \
-H "Content-Type: application/json"
