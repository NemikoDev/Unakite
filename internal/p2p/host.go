package p2p

import (
	"context"
	"fmt"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	"github.com/multiformats/go-multiaddr"
)

// libp2p node using priv key. TCP made random & noise for security
func NewHost(priv crypto.PrivKey) (host.Host, error) {
	h, err := libp2p.New(
		libp2p.Identity(priv),
		libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0"),
		libp2p.Security(noise.ID, noise.New),
	)
	if err != nil {
		return nil, err
	}
	return h, nil
}

// return full addrs that the host is listening to
func FullAddrs(h host.Host) ([]string, error) {
	infos, err := peer.AddrInfoToP2pAddrs(&peer.AddrInfo{
		ID:    h.ID(),
		Addrs: h.Addrs(),
	})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(infos))
	for _, info := range infos {
		out = append(out, info.String())
	}
	return out, nil
}

// Connect to a peer using its full multiaddr and return the peer ID
func Connect(ctx context.Context, h host.Host, fullAddr string) (peer.ID, error) {
	maddr, err := multiaddr.NewMultiaddr(fullAddr)
	if err != nil {
		return "", fmt.Errorf("parsing multiaddr: %w", err)
	}

	info, err := peer.AddrInfoFromP2pAddr(maddr)
	if err != nil {
		return "", fmt.Errorf("extracting peer info: %w", err)
	}

	if err := h.Connect(ctx, *info); err != nil {
		return "", fmt.Errorf("connecting to %s: %w", info.ID, err)
	}
	return info.ID, nil
}
