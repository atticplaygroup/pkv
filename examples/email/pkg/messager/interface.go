package messager

import (
	"context"
	"fmt"
	"log"

	emailpb "github.com/atticplaygroup/pkv/examples/email/pkg/proto/gen/go/examples/email/pkg/proto"
	pb "github.com/atticplaygroup/pkv/pkg/proto/gen/go/kvstore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type IMessager interface {
	SendMetadata(ctx context.Context, sender, recipient string, resourceNames []string) error
	SendContent(ctx context.Context, sender, recipient string, message []byte) (string, error)
	ListMessages(
		ctx context.Context,
		email string,
		pageSize int32,
		pageToken string,
	) ([]*emailpb.EmailMetaMessage, error)
	FetchMessage(
		ctx context.Context,
		emailInfo *emailpb.EmailMetaMessage,
	) ([]byte, error)
	GetMessagerType() *emailpb.MessagerType
	GetVersion() string
}

func NewGrpcClient(host string, port uint16) pb.KvStoreClient {
	grpcOptions := grpc.DialOption(grpc.WithTransportCredentials(insecure.NewCredentials()))
	grpcConn, err := grpc.NewClient(fmt.Sprintf("%s:%d", host, port), grpcOptions)
	if err != nil {
		log.Fatal("cannot dial grpc remote")
	}
	return pb.NewKvStoreClient(grpcConn)
}

func formatResourceParent(m IMessager, recipient string) string {
	return fmt.Sprintf("accounts/%s/streams/email-stream-%d-v%s",
		recipient, *m.GetMessagerType(), m.GetVersion(),
	)
}
