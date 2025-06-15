package email

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/golang-jwt/jwt/v5"

	"github.com/spf13/cobra"
	"google.golang.org/grpc/metadata"
	"gopkg.in/yaml.v3"
)

const (
	mockQuotaUrl      = "http://localhost:8100/quota?max_size=99999&ttl=999999"
	guestTokenUrl     = "http://localhost:8080/v1/guest"
	emailMetaDataHost = "localhost"
	emailMetadataPort = 50051
)

var readCmd = &cobra.Command{
	Use:   "read",
	Short: "Read private messages from server",
	Long:  "",
	Run:   read,
}

func getAuthToken(configPath string) string {
	yamlData, err := os.ReadFile(configPath)
	if err != nil {
		log.Fatalf("failed to parse config file: %v", err)
	}
	var config Config
	err = yaml.Unmarshal(yamlData, &config)
	if err != nil {
		log.Fatalf("failed to parse yaml: %v", err)
	}
	return config.Account.AuthToken
}

// TODO: quota token can be lazy because they can be reused until used up so need to fetch every time
func getMockOrGuestToken(mockTokenUrl string) string {
	response, err := http.Get(mockTokenUrl)
	if err != nil {
		log.Fatalf("cannot get mock prex quota token: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		log.Fatalf("unexpected status when fetching mock quota token: %d", response.StatusCode)
	}
	quotaToken, err := io.ReadAll(response.Body)
	if err != nil {
		log.Fatalf("cannot read body: %v", err)
	}
	return string(quotaToken)
}

func read(cmd *cobra.Command, args []string) {
	configPath, err := cmd.Flags().GetString("config")
	if err != nil {
		log.Fatal("cannot get config path")
	}
	messagerType, err := cmd.Flags().GetString("messager")
	if err != nil {
		log.Fatal("failed to get messager")
	}
	messager1 := getMessager(messagerType)

	authToken := getAuthToken(configPath)
	quotaToken := getMockOrGuestToken(mockQuotaUrl)
	parsedToken, err := jwt.Parse(authToken, nil)
	if err != nil && !errors.Is(err, jwt.ErrTokenUnverifiable) {
		log.Fatalf("could not parse jwt ID token: %v", err)
	}

	email, err := parsedToken.Claims.GetSubject()
	if err != nil {
		log.Fatalf("could not extract subject from jwt id token: %v", err)
	}
	ctx := context.Background()

	// Step 1: Fetch metadata.
	authQuotaMd := metadata.Pairs(
		"authorization", "Bearer "+authToken,
		"x-prex-quota", "Bearer "+quotaToken,
	)
	authQuotaCtx := metadata.NewOutgoingContext(ctx, authQuotaMd)
	emailKeys, err := messager1.ListMessages(authQuotaCtx, email, 9999, "0-0")
	if err != nil {
		log.Fatalf("failed to get email keys: %v", err)
	}

	// Step 2: Fetch email contents.
	var md metadata.MD
	if messagerType == "pgp_e2ee" {
		guestToken := getMockOrGuestToken(guestTokenUrl)
		guestQuotaMd := metadata.Pairs(
			"authorization", "Bearer "+guestToken,
			"x-prex-quota", "Bearer "+quotaToken,
		)
		md = guestQuotaMd
	} else {
		md = authQuotaMd
	}
	contentCtx := metadata.NewOutgoingContext(context.Background(), md)
	for i, emailKey := range emailKeys {
		content, err := messager1.FetchMessage(contentCtx, emailKey)
		if err != nil {
			log.Printf("failed to get email content for %s: %v",
				emailKey.GetContentResourceName(), err)
		}
		fmt.Printf("[%d] email %s:\n%s\n", i, emailKey, content)
	}
}

func init() {
	flags := readCmd.Flags()
	flags.StringP("config", "c", getDefaultConfigPath(), "Config yaml path")
	flags.StringP("messager", "m", "pgp_e2ee", "traditional or pgp_e2ee")

	RootCmd.AddCommand(readCmd)
}
