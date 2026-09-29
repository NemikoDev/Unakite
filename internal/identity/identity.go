package identity

import (
	"crypto/rand"
	"fmt"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

func GenerateEphemeralIdentity() (crypto.PrivKey, peer.ID, error) {
	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)

	if err != nil {
		return nil, "", fmt.Errorf("generating keypair: %w", err)
	}

	id, err := peer.IDFromPrivateKey(priv)

	if err != nil {
		return nil, "", fmt.Errorf("deriving peer id: %w", err)
	}

	return priv, id, nil
}
