package api

import (
	"fmt"

	pb "github.com/atticplaygroup/pkv/pkg/proto/gen/go/kvstore"
	"github.com/redis/go-redis/v9"
)

type Server struct {
	pb.UnimplementedKvStoreServer
	redisClient *redis.Client
	unitPrice   int64
}

func (s *Server) GetRedisClient() *redis.Client {
	return s.redisClient
}

func NewServer(conf *Config) (*Server, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%d", conf.RedisHost, conf.RedisPort),
	})
	server := Server{
		redisClient: rdb,
		unitPrice:   1,
	}
	return &server, nil
}
