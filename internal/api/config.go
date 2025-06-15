package api

import (
	"encoding/base64"
	"log"

	"github.com/spf13/viper"
)

type Config struct {
	RedisHost string `mapstructure:"REDIS_HOST"`
	RedisPort uint16 `mapstructure:"REDIS_PORT"`
	GrpcPort  uint16 `mapstructure:"GRPC_PORT"`

	JwtSecretEncoded               string `mapstructure:"JWT_SECRET"`
	JwtSecret                      []byte
	QuotaAuthorityPublicKeyEncoded string `mapstructure:"QUOTA_AUTHORITY_PUBLIC_KEY"`
	QuotaAuthorityPublicKey        []byte
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
	jwtSecret, err := base64.StdEncoding.DecodeString(config.JwtSecretEncoded)
	if err != nil {
		log.Fatalf("config: failed to parse secret: %v", err)
	}
	config.JwtSecret = jwtSecret
	quotaAuthorityPublicKey, err := base64.StdEncoding.DecodeString(
		config.QuotaAuthorityPublicKeyEncoded)
	if err != nil {
		log.Fatalf("config: failed to parse quota authority public key: %v", err)
	}
	config.QuotaAuthorityPublicKey = quotaAuthorityPublicKey
	return
}
