package email

import (
	"context"
	"fmt"
	"log"

	"github.com/atticplaygroup/pkv/examples/email/pkg/messager"
	"github.com/spf13/cobra"
	"google.golang.org/grpc/metadata"
)

var writeCmd = &cobra.Command{
	Use:   "write",
	Short: "Write private messages to server",
	Long:  "",
	Run:   write,
}

func mockMessage(count string) string {
	return fmt.Sprintf(
		"Subject: %s message\n\n"+
			"Dear Bob, This is my %s message\n", count, count)
}

func getMessager(
	messagerType string,
) messager.IMessager {
	var messager1 messager.IMessager
	host1 := "localhost"
	port1 := uint16(50051)
	// In practice you can set to another endpoint
	host2 := "127.0.0.1"
	port2 := uint16(50051)
	switch messagerType {
	case "pgp_e2ee":
		messager1 = messager.NewPGPE2EEMessager(
			host1, port1,
			host2, port2,
		)
	case "traditional":
		messager1 = messager.NewTraditionalMessager(
			host1, port1,
		)
	default:
		log.Fatalf("unknown messagerType %s", messagerType)
	}
	return messager1
}

func write(cmd *cobra.Command, args []string) {
	messagerType, err := cmd.Flags().GetString("messager")
	if err != nil {
		log.Fatal("failed to get messager")
	}
	messager1 := getMessager(messagerType)
	sender := "alice@op1.com"
	recipient := "bob@op2.com"
	ctx := context.Background()
	quotaToken := getMockSessionToken(mockQuotaUrl)
	md := metadata.Pairs(
		"Authorization", "Bearer "+quotaToken,
	)
	ctx = metadata.NewOutgoingContext(ctx, md)
	firstMessageResourceName, err := messager1.SendContent(
		ctx, sender, recipient, []byte(mockMessage("first")))
	if err != nil {
		log.Fatalf("failed to send message: %v", err)
	}
	secondMessageResourceName, err := messager1.SendContent(
		ctx, sender, recipient, []byte(mockMessage("second")))
	if err != nil {
		log.Fatalf("failed to send message: %v", err)
	}

	messager1.SendMetadata(ctx, sender, recipient, []string{
		firstMessageResourceName,
		secondMessageResourceName,
	})

	fmt.Printf(
		"Successfully sent email, resource names: %s and %s\n",
		firstMessageResourceName,
		secondMessageResourceName,
	)
}

func init() {
	flags := writeCmd.Flags()
	flags.StringP("messager", "m", "pgp_e2ee", "traditional or pgp_e2ee")
	RootCmd.AddCommand(writeCmd)
}
