package messager

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/ProtonMail/gopenpgp/v3/crypto"
)

type IKeyBroker interface {
	LookupPublicKey(email string) (*crypto.Key, error)
	LookupPrivateKey(email string) (*crypto.Key, error)
}
type ToyPGPBroker struct {
	privateKeyStorate map[string]crypto.Key
	publicKeyStorate  map[string]crypto.Key
}

func (b *ToyPGPBroker) LookupPublicKey(email string) (*crypto.Key, error) {
	if key, ok := b.publicKeyStorate[email]; ok {
		return &key, nil
	} else {
		return nil, fmt.Errorf("not found")
	}
}

func (b *ToyPGPBroker) LookupPrivateKey(email string) (*crypto.Key, error) {
	if key, ok := b.privateKeyStorate[email]; ok {
		return &key, nil
	} else {
		return nil, fmt.Errorf("not found")
	}
}

func (b *ToyPGPBroker) newUser(name, email string) {
	pgp := crypto.PGP()
	savePath := fmt.Sprintf("/tmp/toy_pgp_broker/%s.asc", name)
	passphrase := []byte("")
	if armored, err := os.ReadFile(savePath); err == nil {
		priv, err := crypto.NewPrivateKeyFromArmored(string(armored), passphrase)
		if err == nil {
			pub, err := priv.ToPublic()
			if err != nil {
				log.Fatalf("failed to convert to public key: %v", err)
			}
			b.privateKeyStorate[email] = *priv
			b.publicKeyStorate[email] = *pub
			return
		}
	}

	priv, err := pgp.KeyGeneration().
		AddUserId(name, email).
		New().
		GenerateKey()
	if err != nil {
		log.Fatalf("failed to convert to generate private key: %v", err)
	}
	pub, err := priv.ToPublic()
	if err != nil {
		log.Fatalf("failed to convert to public key: %v", err)
	}
	b.privateKeyStorate[email] = *priv
	b.publicKeyStorate[email] = *pub

	parentDir := filepath.Dir(savePath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		log.Fatalf("cannot mkdir: %v", err)
	}
	armor, err := priv.Armor()
	if err != nil {
		log.Fatalf("cannot get armor: %v", err)
	}
	if err := os.WriteFile(savePath, []byte(armor), 0644); err != nil {
		log.Fatalf("cannot write armor to file")
	}
}

func (b *ToyPGPBroker) Init() {
	b.privateKeyStorate = make(map[string]crypto.Key)
	b.publicKeyStorate = make(map[string]crypto.Key)
	b.newUser("alice", "alice@op1.com")
	b.newUser("bob", "bob@op2.com")
}
