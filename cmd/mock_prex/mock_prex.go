package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"time"

	pb "github.com/atticplaygroup/pkv/pkg/proto/gen/go/kvstore/v1"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/spf13/viper"
)

type Config struct {
	TokenSigningSeed       string `mapstructure:"TOKEN_SIGNING_SEED"`
	TokenSigningPrivateKey ed25519.PrivateKey
	QuotaAuthorityDid      string `mapstructure:"QUOTA_AUTHORITY_DID"`
}

func hexToBytes(hexStr string, arrayLength uint8) ([]byte, error) {
	if (len(hexStr) != 2*int(arrayLength)+2) || hexStr[:2] != "0x" {
		return nil, fmt.Errorf(
			"expected hex string for 32 bytes beginning with 0x but got %s", hexStr,
		)
	}

	byteSlice, err := hex.DecodeString(hexStr[2:])
	if err != nil {
		return nil, err
	}
	return byteSlice, nil
}

func HexToBytes32(hexStr string) ([]byte, error) {
	if bytes, err := hexToBytes(hexStr, 32); err != nil {
		return nil, err
	} else {
		return bytes, nil
	}
}

func loadConfig(name string, path string) (config Config) {
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
	seed, err := HexToBytes32(config.TokenSigningSeed)
	if err != nil {
		log.Fatalf("failed to parse TokenSigningSeed %s: %v", config.TokenSigningSeed, err)
	}
	if len(seed) != ed25519.SeedSize {
		log.Fatalf("expect seed to have len %d but got %d: %v", ed25519.SeedSize, len(seed), seed)
	}
	config.TokenSigningPrivateKey = ed25519.NewKeyFromSeed(seed)
	fmt.Printf("pubkey: %s\n", base64.StdEncoding.EncodeToString(
		config.TokenSigningPrivateKey.Public().(ed25519.PublicKey)))
	issuerDid = config.QuotaAuthorityDid
	return config
}

var config Config
var issuerDid string

func main() {
	config = loadConfig(".env", "/workspaces/pkv")

	http.HandleFunc("/quota", handleQuota)

	servicePort := 8100
	fmt.Printf("Usage like: `curl http://localhost:%d/quota?max_size=99999&ttl=999999`\n", servicePort)
	log.Printf("Server started at :%d\n", servicePort)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", servicePort), nil))
}

func handleQuota(w http.ResponseWriter, r *http.Request) {
	claim := getClaims()
	claims := QuotaClaims{
		Usage:            pb.JwtUsage_JWT_USAGE_CREATE_SESSION,
		Quantity:         1_000_000,
		RegisteredClaims: &claim,
	}

	token := jwt.NewWithClaims(&jwt.SigningMethodEd25519{}, claims)
	jwtSigned, err := token.SignedString(config.TokenSigningPrivateKey)
	if err != nil {
		panic(err)
	}
	fmt.Fprintf(w, "%s", jwtSigned)
}

type QuotaClaims struct {
	Usage    pb.JwtUsage `json:"usage"`
	Quantity int64       `json:"quantity"`
	*jwt.RegisteredClaims
}

func getClaims() jwt.RegisteredClaims {
	serviceIdentifier := issuerDid
	return jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(4 * time.Minute)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		NotBefore: jwt.NewNumericDate(time.Now()),
		Issuer:    serviceIdentifier,
		ID:        uuid.NewString(),
		Audience:  []string{serviceIdentifier},
	}
}
