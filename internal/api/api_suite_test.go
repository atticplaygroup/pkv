package api_test

import (
	"log"
	"testing"

	"github.com/atticplaygroup/pkv/internal/api"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestApi(t *testing.T) {
	RegisterFailHandler(Fail)
	JwtSeed = "HbFdKCKTGkzcWKMPWmHKjW/Ii/wpcKTyD+8QIxw3Gc0="
	conf := api.LoadConfig(".env", "../..")
	JwtHS256Secret = conf.JwtSecretEncoded

	conn, err := grpc.NewClient(
		"localhost:50051",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	GrpcClient = conn

	RunSpecs(t, "Store Suite")
}
