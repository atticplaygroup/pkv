package middleware

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/mr-tron/base58/base58"
	"github.com/redis/go-redis/v9"

	pb "github.com/atticplaygroup/pkv/pkg/proto/gen/go/kvstore/v1"
)

type ISessionManager interface {
	CreateSession(ctx context.Context, jti string, balance int64, ttl time.Duration) (*pb.Session, error)
	DeductSessionBalance(ctx context.Context, sessionId string, amount int64) (*pb.Session, error)
}

type RedisSessionManager struct {
	redisClient *redis.Client
	sessionSalt []byte
}

func NewRedisSessionManager(redisClient *redis.Client) *RedisSessionManager {
	sessionSalt := make([]byte, 32)
	_, err := rand.Read(sessionSalt)
	if err != nil {
		log.Fatal("failed to generate random sessionSalt")
	}
	return &RedisSessionManager{
		redisClient: redisClient,
		sessionSalt: sessionSalt,
	}
}

func (s *RedisSessionManager) hashToSessionId(jti string) string {
	buf := append(s.sessionSalt, []byte(jti)...)
	hashed := sha256.Sum256(buf)
	return base58.Encode(hashed[:])
}

func (s *RedisSessionManager) CreateSession(ctx context.Context, jti string, balance int64, ttl time.Duration) (*pb.Session, error) {
	sessionId := s.hashToSessionId(jti)
	session := &pb.Session{
		SessionId: sessionId,
		Balance:   balance,
	}
	redisKey := fmt.Sprintf("session:%s", sessionId)

	result, err := s.redisClient.Do(
		ctx,
		"SET", redisKey, session.GetBalance(), "NX", "GET",
	).Result()
	if err == redis.Nil {
		return session, nil
	}
	if err != nil {
		return nil, err
	}
	latestBalance, err := strconv.Atoi(result.(string))
	if err != nil {
		return nil, fmt.Errorf("failed to parse balance: %s", result)
	}
	return &pb.Session{
		SessionId: session.GetSessionId(),
		Balance:   int64(latestBalance),
	}, nil
}

type SessionJwtClaims struct {
	*jwt.RegisteredClaims
	Usage pb.JwtUsage `json:"usage"`
}

type CreateSessionJwtClaims struct {
	*SessionJwtClaims
	Quantity int64 `json:"quantity"`
}

func (s *RedisSessionManager) DeductSessionBalance(ctx context.Context, sessionId string, amount int64) (*pb.Session, error) {
	if amount <= 0 {
		return nil, fmt.Errorf("amount must be positive but got %d", amount)
	}
	redisKey := fmt.Sprintf("session:%s", sessionId)
	newBalance, err := s.redisClient.DecrBy(ctx, redisKey, amount).Result()
	if err == redis.Nil {
		return nil, fmt.Errorf("session not found or expired")
	} else if err != nil {
		return nil, err
	}
	if newBalance < 0 {
		return nil, fmt.Errorf("insufficient balance: %d", newBalance)
	}
	return &pb.Session{
		Balance:   newBalance,
		SessionId: sessionId,
	}, nil
}
