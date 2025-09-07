package api

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/atticplaygroup/pkv/pkg/middleware"
	pb "github.com/atticplaygroup/pkv/pkg/proto/gen/go/kvstore/v1"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
)

// TODO: make ttl stream specific
const streamTtl = time.Duration(7 * 24 * 3600 * time.Second)

// TODO: batchify me
func (s *Server) CreateStreamValue(
	ctx context.Context, connectReq *connect.Request[pb.CreateStreamValueRequest],
) (*connect.Response[pb.CreateStreamValueResponse], error) {
	req := connectReq.Msg
	streamID := req.GetParent()
	// Retentions are paid by value writers.
	// Stream's retention is also set to streamTtl.
	// When future writes arrive, it will clean > streamTtl old entries.
	// An entry may exist at most for 2 * streamTtl if no future writes arrived.
	// Because by that time the entire stream will be deleted by the stream's TTL.
	minID := fmt.Sprintf("%d-0", (time.Now().Add(-streamTtl)).Unix()*1000)
	entryId, err := s.redisClient.XAdd(
		ctx,
		&redis.XAddArgs{
			Stream: streamID,
			MinID:  minID,
			Approx: true,
			Values: map[string]interface{}{
				"value": req.GetValue(),
			},
		},
	).Result()
	if err != nil {
		return nil, status.Error(
			codes.Internal,
			"failed to set value",
		)
	}
	if err := s.redisClient.Expire(ctx, streamID, streamTtl).Err(); err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"failed to refresh stream TTL: %v",
			err,
		)
	}
	return connect.NewResponse(&pb.CreateStreamValueResponse{
		Name: fmt.Sprintf("%s/values/%s", req.GetParent(), entryId),
		Ttl:  durationpb.New(streamTtl),
	}), nil
}

func parseXMessage(xMessage *redis.XMessage) (*pb.StreamValueInfo, error) {
	rawValue, ok := xMessage.Values["value"]
	if !ok {
		return nil, fmt.Errorf("value field not found from XMessage")
	}
	value, ok := rawValue.(string)
	if !ok {
		return nil, fmt.Errorf("failed to parse value to bytes")
	}
	return &pb.StreamValueInfo{
		Value:         []byte(value),
		StreamEntryId: xMessage.ID,
	}, nil
}

type EntryID struct {
	Timestamp  int64
	SequenceID int64
}

func parseEntryID(entryID string) (*EntryID, error) {
	splitted := strings.Split(entryID, "-")
	if len(splitted) != 2 {
		return nil, fmt.Errorf("corrupted entry ID: %v", splitted)
	}
	timestamp, err := strconv.Atoi(splitted[0])
	if err != nil {
		return nil, fmt.Errorf("failed to parse timestamp %v", err)
	}
	sequenceID, err := strconv.Atoi(splitted[1])
	if err != nil {
		return nil, fmt.Errorf("failed to parse sequence ID %v", err)
	}
	return &EntryID{
		Timestamp:  int64(timestamp),
		SequenceID: int64(sequenceID),
	}, nil
}

func getMaxEntryID(a, b string) (string, error) {
	aEntryID, err := parseEntryID(a)
	if err != nil {
		return "", err
	}
	bEntryID, err := parseEntryID(b)
	if err != nil {
		return "", err
	}
	if aEntryID.Timestamp < bEntryID.Timestamp ||
		(aEntryID.Timestamp == bEntryID.Timestamp &&
			aEntryID.SequenceID < bEntryID.SequenceID) {
		return b, nil
	} else {
		return a, nil
	}
}

func (s *Server) ListStreamValues(
	ctx context.Context, connectReq *connect.Request[pb.ListStreamValuesRequest],
) (*connect.Response[pb.ListStreamValuesResponse], error) {
	req := connectReq.Msg
	streamID := req.GetParent()
	fields, err := middleware.ParseResourceName(req.GetParent(), []string{
		"accounts", "streams",
	})
	if err != nil {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"failed to parse resource name: %v",
			err,
		)
	}
	if err := s.ensureAuthToken(req.GetAuthToken(), fields[0]); err != nil {
		return nil, err
	}
	xMessages, err := s.redisClient.XRangeN(
		ctx,
		streamID,
		req.GetPageToken(),
		"+",
		int64(req.GetPageSize()),
	).Result()
	if err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"failed to get stream: %v",
			err,
		)
	}
	if len(xMessages) == 0 {
		return connect.NewResponse(&pb.ListStreamValuesResponse{
			StreamValueInfo: nil,
			PageToken:       "0-0",
		}), nil
	}

	ret := make([]*pb.StreamValueInfo, 0)
	maxEntryID := "0-0"
	for _, xMessage := range xMessages {
		streamValueInfo, err := parseXMessage(&xMessage)
		if err != nil {
			return nil, status.Errorf(
				codes.Internal,
				"failed to parse xMessage: %v",
				err,
			)
		}
		ret = append(ret, streamValueInfo)
		maxEntryID, err = getMaxEntryID(maxEntryID, xMessage.ID)
		if err != nil {
			return nil, status.Errorf(
				codes.Internal,
				"failed to compare entry ID: %v",
				err,
			)
		}
	}
	return connect.NewResponse(&pb.ListStreamValuesResponse{
		StreamValueInfo: ret,
		PageToken:       maxEntryID,
	}), nil
}

func (s *Server) ensureAuthToken(authToken string, expectedSubject string) error {
	claims, err := s.authmanager.VerifyAndParseJwt(authToken, &jwt.RegisteredClaims{}, true)
	if err != nil {
		return status.Errorf(
			codes.PermissionDenied,
			"failed to parse auth token in request: %v",
			err,
		)
	}
	subject, err := claims.GetSubject()
	if err != nil {
		return status.Errorf(
			codes.PermissionDenied,
			"failed to parse subject in auth token: %v",
			err,
		)
	}
	if subject != expectedSubject {
		return status.Errorf(
			codes.PermissionDenied,
			"auth token subject %s not matching resource owner %s",
			subject,
			expectedSubject,
		)
	}
	return nil
}

func (s *Server) GetStreamValue(
	ctx context.Context, connectReq *connect.Request[pb.GetStreamValueRequest],
) (*connect.Response[pb.GetStreamValueResponse], error) {
	req := connectReq.Msg
	fields, err := middleware.ParseResourceName(req.GetName(), []string{
		"accounts", "streams", "values",
	})
	if err != nil {
		return nil, status.Errorf(
			codes.InvalidArgument,
			"failed to parse resource name: %v",
			err,
		)
	}
	if err := s.ensureAuthToken(req.GetAuthToken(), fields[0]); err != nil {
		return nil, err
	}
	streamID := fmt.Sprintf("accounts/%s/streams/%s", fields[0], fields[1])
	xStream, err := s.redisClient.XRead(ctx, &redis.XReadArgs{
		Streams: []string{streamID},
		ID:      fields[2],
		Count:   1,
	}).Result()
	if err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"failed to get value: %v",
			err,
		)
	}
	if len(xStream) > 1 {
		return nil, status.Errorf(
			codes.Internal,
			"got %d streams but expected 1: %v",
			len(xStream),
			xStream,
		)
	}
	if len(xStream) == 0 || len(xStream[0].Messages) == 0 {
		return nil, status.Error(
			codes.NotFound,
			"stream or message not found",
		)
	}

	streamValueInfo, err := parseXMessage(&xStream[0].Messages[0])
	if err != nil {
		return nil, status.Errorf(
			codes.Internal,
			"failed to parse xMessage: %v",
			err,
		)
	}
	return connect.NewResponse(&pb.GetStreamValueResponse{
		StreamValueInfo: streamValueInfo,
	}), nil
}
