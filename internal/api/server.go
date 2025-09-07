package api

import (
	"crypto/ed25519"
	"fmt"

	"github.com/atticplaygroup/pkv/pkg/middleware"
	"github.com/redis/go-redis/v9"
)

type Server struct {
	config         *Config
	redisClient    *redis.Client
	sessionManager middleware.ISessionManager
	authmanager    middleware.IAuthManager
	unitPrice      int64
}

func (s *Server) GetRedisClient() *redis.Client {
	return s.redisClient
}

func (s *Server) GetAuthManager() middleware.IAuthManager {
	return s.authmanager
}

func NewServer(conf *Config) (*Server, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%d", conf.RedisHost, conf.RedisPort),
	})
	server := Server{
		config:         conf,
		redisClient:    rdb,
		unitPrice:      1,
		sessionManager: middleware.NewRedisSessionManager(rdb),
		authmanager: middleware.NewStaticAuthManager(
			conf.JwtSecret,
			map[string]ed25519.PublicKey{
				conf.QuotaAuthorityDid: conf.QuotaAuthorityPublicKey,
			},
			"did:example:pkv",
		),
	}
	return &server, nil
}
