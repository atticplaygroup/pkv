package messager

import (
	"context"
	"fmt"

	emailpb "github.com/atticplaygroup/pkv/examples/email/pkg/proto/gen/go/pkg/proto"
	pb "github.com/atticplaygroup/pkv/pkg/proto/gen/go/kvstore/v1"
	"google.golang.org/protobuf/proto"
)

type TraditionalMessager struct {
	metadataServiceHost string
	metadataServicePort uint16

	self IMessager
}

func NewTraditionalMessager(
	host string, port uint16,
) *TraditionalMessager {
	ret := &TraditionalMessager{
		metadataServiceHost: host,
		metadataServicePort: port,
	}
	ret.self = ret
	return ret
}

func (m *TraditionalMessager) DoSendContent(
	ctx context.Context, grpcClient pb.KvStoreServiceClient, sender string, message []byte,
) (string, error) {
	contentResp, err := grpcClient.CreateValue(
		ctx, &pb.CreateValueRequest{
			Value: []byte(message),
		},
	)
	if err != nil {
		return "", fmt.Errorf("failed to set content: %v", err)
	}
	return contentResp.GetName(), nil
}

func (m *TraditionalMessager) SendContent(
	ctx context.Context, sender, recipient string, message []byte,
) (string, error) {
	grpcClient := NewGrpcClient(m.metadataServiceHost, m.metadataServicePort)
	return m.DoSendContent(ctx, grpcClient, sender, message)
}

func (m *TraditionalMessager) SendMetadata(
	ctx context.Context, sender, recipient string, resourceNames []string,
) error {
	parent := formatResourceParent(m.self, recipient)
	grpcClient := NewGrpcClient(
		m.metadataServiceHost,
		m.metadataServicePort,
	)
	for _, resourceName := range resourceNames {
		emailMetadata, err := proto.Marshal(&emailpb.EmailMetaMessage{
			Host:                m.metadataServiceHost,
			Port:                uint32(m.metadataServicePort),
			Sender:              sender,
			Recipient:           recipient,
			ContentResourceName: resourceName,
		})
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %v", err)
		}
		_, err = grpcClient.CreateStreamValue(
			ctx, &pb.CreateStreamValueRequest{
				Parent: parent,
				Value:  emailMetadata,
			})
		if err != nil {
			return fmt.Errorf("failed to sent metadata: %v", err)
		}
	}

	return nil
}

func (m *TraditionalMessager) GetMessagerType() *emailpb.MessagerType {
	return emailpb.MessagerType_TRADITIONAL.Enum()
}

func (m *TraditionalMessager) GetVersion() string {
	return "0.1.0"
}

func parseEmailInfo(
	value []byte,
) *emailpb.EmailMetaMessage {
	var ret emailpb.EmailMetaMessage
	proto.Unmarshal(value, &ret)
	return &ret
}

func (m *TraditionalMessager) ListMessages(
	ctx context.Context,
	email string,
	pageSize int32,
	pageToken string,
	authToken string,
) ([]*emailpb.EmailMetaMessage, error) {
	parent := formatResourceParent(m.self, email)
	grpcClient := NewGrpcClient(m.metadataServiceHost, m.metadataServicePort)
	resp, err := grpcClient.ListStreamValues(
		ctx, &pb.ListStreamValuesRequest{
			Parent:    parent,
			PageSize:  pageSize,
			PageToken: pageToken,
			AuthToken: authToken,
		},
	)
	if err != nil {
		return nil, err
	}
	ret := make([]*emailpb.EmailMetaMessage, 0)
	for _, info := range resp.StreamValueInfo {
		ret = append(ret, parseEmailInfo(info.GetValue()))
	}
	return ret, nil
}

func (m *TraditionalMessager) DoFetchMessage(
	ctx context.Context,
	emailInfo *emailpb.EmailMetaMessage,
) ([]byte, error) {
	grpcClient := NewGrpcClient(m.metadataServiceHost, m.metadataServicePort)
	resp, err := grpcClient.GetValue(ctx, &pb.GetValueRequest{
		Name: emailInfo.GetContentResourceName(),
	})
	if err != nil {
		return nil, err
	}
	return resp.GetValue(), nil
}

func (m *TraditionalMessager) FetchMessage(
	ctx context.Context,
	emailInfo *emailpb.EmailMetaMessage,
) ([]byte, error) {
	return m.DoFetchMessage(ctx, emailInfo)
}
