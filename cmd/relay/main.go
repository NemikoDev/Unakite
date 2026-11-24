package main

import (
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	relayv2 "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
)

func main() {
	listen := flag.String("listen", "/ip4/0.0.0.0/tcp/4001", "multiaddr to listen on")
	keyFile := flag.String("key", "relay.key", "file holding the relay's identity (created on first run)")
	flag.Parse()

	priv, err := loadOrCreateKey(*keyFile)
	if err != nil {
		log.Fatal(err)
	}

	h, err := libp2p.New(
		libp2p.Identity(priv),
		libp2p.ListenAddrStrings(*listen),
		libp2p.Security(noise.ID, noise.New),
		libp2p.ForceReachabilityPublic(),
		libp2p.EnableRelayService(relayv2.WithInfiniteLimits()),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer h.Close()

	log.Printf("relay running, peer ID %s", h.ID())
	for _, a := range h.Addrs() {
		log.Printf("  listening on %s/p2p/%s", a, h.ID())
	}
	log.Printf("clients need: UNAKITE_RELAY_ADDR=/ip4/<PUBLIC-IP>/tcp/<port>/p2p/%s", h.ID())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
}

// loads the relay's private key from the specified file, or creates a new one if it doesn't exist
func loadOrCreateKey(path string) (crypto.PrivKey, error) {
	raw, err := os.ReadFile(path)
	switch {
	case err == nil:
		return crypto.UnmarshalPrivateKey(raw)
	case !errors.Is(err, fs.ErrNotExist):
		return nil, err
	}
	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, err
	}
	raw, err = crypto.MarshalPrivateKey(priv)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return nil, fmt.Errorf("saving relay key: %w", err)
	}
	return priv, nil
}
