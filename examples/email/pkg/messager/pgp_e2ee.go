package messager

import (
	"context"
	"fmt"

	"github.com/ProtonMail/gopenpgp/v3/crypto"
	emailpb "github.com/atticplaygroup/pkv/examples/email/pkg/proto/gen/go/pkg/proto"
)

type PGPE2EEMessager struct {
	contentServiceHost string
	contentServicePort uint16
	*TraditionalMessager

	keyBroker IKeyBroker
}

func NewPGPE2EEMessager(
	metadataServiceHost string,
	metadataServicePort uint16,
	contentServiceHost string,
	contentServicePort uint16,
) *PGPE2EEMessager {
	m := &PGPE2EEMessager{
		TraditionalMessager: &TraditionalMessager{
			metadataServiceHost: metadataServiceHost,
			metadataServicePort: metadataServicePort,
		},
		contentServiceHost: contentServiceHost,
		contentServicePort: contentServicePort,
	}
	m.self = m
	m.Init()
	return m
}

func (m *PGPE2EEMessager) Init() {
	toyPgpBroker := new(ToyPGPBroker)
	toyPgpBroker.Init()
	m.keyBroker = toyPgpBroker
}

func (m *PGPE2EEMessager) SendContent(
	ctx context.Context, sender, recipient string, message []byte,
) (string, error) {
	grpcClient := NewGrpcClient(m.contentServiceHost, m.contentServicePort)

	senderPrivateKey, err := m.keyBroker.LookupPrivateKey(sender)
	if err != nil {
		return "", fmt.Errorf("sender private key lookup failed: %v", err)
	}
	recipientPublicKey, err := m.keyBroker.LookupPublicKey(recipient)
	if err != nil {
		return "", fmt.Errorf("recipient public key lookup failed: %v", err)
	}
	pgp := crypto.PGP()
	encHandle, err := pgp.Encryption().
		Recipient(recipientPublicKey).
		SigningKey(senderPrivateKey).
		New()
	if err != nil {
		return "", err
	}
	pgpMessage, err := encHandle.Encrypt([]byte(message))
	if err != nil {
		return "", err
	}
	armored, err := pgpMessage.ArmorBytes()
	if err != nil {
		return "", err
	}

	return m.DoSendContent(ctx, grpcClient, sender, armored)
}
func (m *PGPE2EEMessager) FetchMessage(
	ctx context.Context,
	emailInfo *emailpb.EmailMetaMessage,
) ([]byte, error) {
	rawMessage, err := m.DoFetchMessage(ctx, emailInfo)
	if err != nil {
		return nil, err
	}
	senderPublicKey, err := m.keyBroker.LookupPublicKey(emailInfo.GetSender())
	if err != nil {
		return nil, fmt.Errorf("sender public key lookup failed: %v", err)
	}
	recipientPrivateKey, err := m.keyBroker.LookupPrivateKey(emailInfo.GetRecipient())
	if err != nil {
		return nil, fmt.Errorf("recipient private key lookup failed: %v", err)
	}

	pgp := crypto.PGP()
	decHandle, err := pgp.Decryption().
		DecryptionKey(recipientPrivateKey).
		VerificationKey(senderPublicKey).
		New()
	if err != nil {
		return nil, fmt.Errorf("failed to init decHandle: %v", err)
	}
	decrypted, err := decHandle.Decrypt(rawMessage, crypto.Armor)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt message: %v", err)
	}
	if sigErr := decrypted.SignatureError(); sigErr != nil {
		return decrypted.Bytes(), fmt.Errorf("message verification failed: %v", err)
	}
	return decrypted.Bytes(), nil
}

func (m *PGPE2EEMessager) GetMessagerType() *emailpb.MessagerType {
	return emailpb.MessagerType_PGP_E2EE.Enum()
}

func (m *PGPE2EEMessager) GetVersion() string {
	return "0.1.0"
}
