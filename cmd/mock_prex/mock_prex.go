package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/spf13/viper"
)

type Config struct {
	signingSeedEncoded string `mapstructure:"SIGNING_SEED"`
	privateKey         ed25519.PrivateKey
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
	config.signingSeedEncoded = viper.GetString("SIGNING_SEED")
	config.privateKey = ed25519.NewKeyFromSeed(
		mustDecodeBytes(config.signingSeedEncoded))
	fmt.Printf("pubkey: %s\n", base64.StdEncoding.EncodeToString(
		config.privateKey.Public().(ed25519.PublicKey)))
	return config
}

var config Config

func main() {
	config = loadConfig(".env", "/workspaces/pkv")

	http.HandleFunc("/quota", handleQuota)

	servicePort := 8100
	fmt.Printf("Usage like: `curl http://localhost:%d/quota?max_size=99999&ttl=999999`\n", servicePort)
	log.Printf("Server started at :%d\n", servicePort)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", servicePort), nil))
}

func handleQuota(w http.ResponseWriter, r *http.Request) {
	ttlStr := r.FormValue("ttl")
	ttl, err := strconv.Atoi(ttlStr)
	if err != nil {
		http.Error(w, "ttl parameter not found", http.StatusBadRequest)
	}
	maxSizeStr := r.FormValue("max_size")
	maxSize, err := strconv.Atoi(maxSizeStr)
	if err != nil {
		http.Error(w, "max_size parameter not found", http.StatusBadRequest)
	}
	claim := getClaims("guest")
	claims := QuotaClaims{
		Ttl:              ttl,
		MaxSize:          maxSize,
		RegisteredClaims: &claim,
	}

	token := jwt.NewWithClaims(&jwt.SigningMethodEd25519{}, claims)
	jwtSigned, err := token.SignedString(config.privateKey)
	if err != nil {
		panic(err)
	}
	fmt.Fprintf(w, "%s", jwtSigned)
}

type QuotaClaims struct {
	Ttl     int `json:"ttl"`
	MaxSize int `json:"max_size"`
	*jwt.RegisteredClaims
}

func getClaims(subject string) jwt.RegisteredClaims {
	serviceIdentifier := "myself"
	return jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(4 * time.Minute)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		NotBefore: jwt.NewNumericDate(time.Now()),
		Issuer:    serviceIdentifier,
		Subject:   subject,
		ID:        uuid.NewString(),
		Audience:  []string{serviceIdentifier},
	}
}

func mustDecodeBytes(encoded string) []byte {
	ret, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		log.Fatalf("config: failed to parse base64: %v", err)
	}
	return ret
}
