package api

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/mr-tron/base58"
	"github.com/spf13/viper"
	"golang.org/x/crypto/hkdf"
)

type Config struct {
	RedisHost string `mapstructure:"REDIS_HOST"`
	RedisPort uint16 `mapstructure:"REDIS_PORT"`
	GrpcPort  uint16 `mapstructure:"GRPC_PORT"`

	TokenTtl          time.Duration `mapstructure:"TOKEN_TTL"`
	SelfIdentifier    string        `mapstructure:"SELF_IDENTIFIER"`
	TokenSigningKeyId string

	SecretSeedEncoded         string `mapstructure:"SECRET_SEED"`
	ExchangeAccountPrivateKey ed25519.PrivateKey
	Libp2pPrivateKey          crypto.PrivKey
	JwtSecret                 []byte
	QuotaAuthorityDid         string `mapstructure:"QUOTA_AUTHORITY_DID"`
	QuotaAuthorityPublicKey   []byte
}

func mustParseEd25519DidKey(didString string) []byte {
	base58Str, found := strings.CutPrefix(didString, "did:key:z")
	if !found {
		log.Fatalf("invalid did key: %s", didString)
	}
	didBytes, err := base58.Decode(base58Str)
	if err != nil {
		log.Fatalf("failed to decode: %s", err.Error())
	}
	if len(didBytes) != 34 || didBytes[0] != 0xed || didBytes[1] != 0x01 {
		log.Fatalf("did is not ed25519: %v", didBytes)
	}
	return didBytes[2:]
}

func DeriveKey(seed []byte, info string) ([]byte, error) {
	hash := sha256.New
	hkdf := hkdf.New(hash, seed, nil, []byte(info))
	secret := make([]byte, 32)
	if _, err := io.ReadFull(hkdf, secret); err != nil {
		return nil, fmt.Errorf("failed to derive secret %s: %v", info, err)
	}
	return secret, nil
}

func LoadConfig(name string, path string) (config Config) {
	viper.AddConfigPath(path)
	viper.SetConfigName(name)
	viper.SetConfigType("env")

	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := viper.Unmarshal(&config); err != nil {
		log.Fatalf("config: %v", err)
	}
	seed, err := base64.StdEncoding.DecodeString(config.SecretSeedEncoded)
	if err != nil {
		log.Fatalf("config: failed to parse secret: %v", err)
	}
	config.JwtSecret, err = DeriveKey(seed, "JwtSecret")
	if err != nil {
		log.Fatalf("config: failed to derive key: %v", err)
	}
	config.TokenSigningKeyId = base58.Encode(sha256.New().Sum(config.JwtSecret))
	exchangeKeySeed, err := DeriveKey(seed, "ExchangeKey")
	if err != nil {
		log.Fatalf("config: failed to derive key: %v", err)
	}
	config.ExchangeAccountPrivateKey = ed25519.NewKeyFromSeed(exchangeKeySeed)
	libp2pSeed, err := DeriveKey(seed, "Libp2p")
	if err != nil {
		log.Fatalf("config: failed to derive key: %v", err)
	}
	config.Libp2pPrivateKey, _, err = crypto.GenerateEd25519Key(bytes.NewReader(libp2pSeed))
	if err != nil {
		log.Fatalf("config: failed to generate libp2p key: %v", err)
	}
	config.QuotaAuthorityPublicKey = mustParseEd25519DidKey(config.QuotaAuthorityDid)
	return
}
