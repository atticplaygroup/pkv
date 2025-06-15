package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/viper"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type Config struct {
	clientID     string `mapstructure:"CLIENT_ID"`
	clientSecret string `mapstructure:"CLIENT_SECRET"`
	redirectURL  string `mapstructure:"REDIRECT_URL"`
	servicePort  uint16 `mapstructure:"SERVICE_PORT"`
	redisHost    string `mapstructure:"REDIS_HOST"`
	redisPort    uint16 `mapstructure:"REDIS_PORT"`

	oauth2Config oauth2.Config

	jwtSecretEncoded string `mapstructure:"JWT_SECRET"`
	jwtSecret        []byte

	isTest bool `mapstructure:"IS_TEST"`
}

func mustDecodeBytes(encoded string) []byte {
	ret, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		log.Fatalf("config: failed to parse base64: %v", err)
	}
	return ret
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
	config.servicePort = viper.GetUint16("SERVICE_PORT")
	config.jwtSecret = mustDecodeBytes(viper.GetString("JWT_SECRET"))
	config.isTest = viper.GetBool("IS_TEST")
	config.redisHost = viper.GetString("REDIS_HOST")
	config.redisPort = viper.GetUint16("REDIS_PORT")

	endpoint := google.Endpoint
	if config.isTest {
		endpoint = oauth2.Endpoint{
			TokenURL:  "http://localhost:8081/oauth2/token",
			AuthURL:   "http://localhost:8081/oauth2/auth",
			AuthStyle: oauth2.AuthStyleInParams,
		}
	}
	config.oauth2Config = oauth2.Config{
		ClientID:     viper.GetString("CLIENT_ID"),
		ClientSecret: viper.GetString("CLIENT_SECRET"),
		RedirectURL:  viper.GetString("REDIRECT_URL"),
		Scopes:       []string{"openid", "email"},
		Endpoint:     endpoint,
	}
	return config
}

var config Config
var redisClient *redis.Client

func main() {
	config = loadConfig(".env", "/workspaces/pkv")

	redisClient = redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%d", config.redisHost, config.redisPort),
	})

	http.HandleFunc("/", handleHome)
	http.HandleFunc("/v1/login", handleLogin)
	http.HandleFunc("/v1/guest", handleGuestLogin)
	http.HandleFunc("/v1/callback", handleCallback)

	log.Printf("Server started at :%d\n", config.servicePort)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", config.servicePort), nil))
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	html := `<html><body><a href="/login">Login with Google</a></body></html>`
	fmt.Fprint(w, html)
}

func handleGuestLogin(w http.ResponseWriter, _ *http.Request) {
	jwt, err := generateJwtHs256("guest")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprint(w, jwt)
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	verifier := oauth2.GenerateVerifier()
	state := uuid.NewString()
	ttl := 300 * time.Second
	redisKey := fmt.Sprintf("verifier/%s", state)
	if err := redisClient.Set(ctx, redisKey, verifier, ttl).Err(); err != nil {
		http.Error(w, "failed to save code verifier", http.StatusInternalServerError)
	}
	url := config.oauth2Config.AuthCodeURL(
		state, oauth2.AccessTypeOnline, oauth2.S256ChallengeOption(verifier))
	http.Redirect(w, r, url, http.StatusFound)
}

func getUserSub(ctx context.Context, token *oauth2.Token) (string, error) {
	client := config.oauth2Config.Client(ctx, token)
	userInfo, err := getUserInfo(client)
	if err != nil {
		return "", fmt.Errorf("failed to get user info: %v", err)
	}
	email, ok := userInfo["email"].(string)
	if ok {
		return email, nil
	}
	return "", fmt.Errorf("failed to parse email field")
}

func getUserInfo(client *http.Client) (map[string]interface{}, error) {
	userInfoURL := "https://oauth2.googleapis.com/tokeninfo"
	if config.isTest {
		userInfoURL = "http://localhost:8081/tokeninfo"
	}
	resp, err := client.Get(userInfoURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get user info: %s", resp.Status)
	}

	var userInfo map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, err
	}

	return userInfo, nil
}

func handleCallback(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	redisKey := fmt.Sprintf("verifier/%s", state)
	verifier, err := redisClient.GetDel(ctx, redisKey).Result()
	if err != nil {
		http.Error(w, "failed to get verifier: "+err.Error(), http.StatusInternalServerError)
		return
	}

	token, err := config.oauth2Config.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		http.Error(w, "failed to exchange token: "+err.Error(), http.StatusInternalServerError)
		return
	}
	subject, err := getUserSub(ctx, token)
	if err != nil {
		http.Error(w, "failed to get user sub: "+err.Error(), http.StatusInternalServerError)
		return
	}
	jwt, err := generateJwtHs256(subject)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprint(w, jwt)
}

func generateJwtHs256(subject string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, getClaims(subject))
	jwt, err := token.SignedString(config.jwtSecret)
	if err != nil {
		log.Printf("failed to sign jwt: %v", err)
		return "", err
	}
	return jwt, nil
}

func getClaims(subject string) jwt.RegisteredClaims {
	serviceIdentifier := "myself"
	return jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		NotBefore: jwt.NewNumericDate(time.Now()),
		Issuer:    serviceIdentifier,
		Subject:   subject,
		ID:        uuid.NewString(),
		Audience:  []string{serviceIdentifier},
	}
}
