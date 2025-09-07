FROM mcr.microsoft.com/devcontainers/go:1-1.24-bookworm AS dev-builder
ARG SQL_MIGRATE_VERSION=1.7.0
RUN cd /tmp && \
    wget https://github.com/rubenv/sql-migrate/archive/refs/tags/v${SQL_MIGRATE_VERSION}.zip && \
    unzip v${SQL_MIGRATE_VERSION}.zip && \
    pwd && \
    cd sql-migrate-${SQL_MIGRATE_VERSION} && \
    go mod tidy && go mod vendor
RUN cd /tmp/sql-migrate-${SQL_MIGRATE_VERSION} && \
    GOOS=linux GOARCH=amd64 CGO_ENABLED=0 GO111MODULE=on go build \
    -v \
    -mod vendor \
    -o ./bin/sql-migrate \
        ./sql-migrate && mv ./bin/sql-migrate /tmp
RUN cd /tmp && git clone https://github.com/gitleaks/gitleaks.git && \
    cd /tmp/gitleaks && \
    make build

FROM mcr.microsoft.com/devcontainers/go:1-1.24-bookworm
COPY --from=dev-builder /tmp/sql-migrate /usr/local/bin/sql-migrate
COPY --from=dev-builder /tmp/gitleaks/gitleaks /usr/local/bin/gitleaks
RUN chmod +x /usr/local/bin/sql-migrate
RUN apt-get update && apt-get install -y protobuf-compiler python3.11-venv python3-pip
USER vscode
RUN go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest && \
    go install github.com/onsi/ginkgo/v2/ginkgo@latest && \
    go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest && \
    go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@latest && \
    go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest && \
    go install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest && \
    go install github.com/bufbuild/buf/cmd/buf@latest && \
    go install github.com/hashicorp/protoc-gen-go-binary@latest && \
    go install github.com/spf13/cobra-cli@latest && \
    go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
USER root
WORKDIR /workspaces/pkv
