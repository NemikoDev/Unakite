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
func NewHost(priv crypto.PrivKey, opts ...libp2p.Option) (host.Host, error) {
	base := []libp2p.Option{
		libp2p.Identity(priv),
		libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0"),
		libp2p.Security(noise.ID, noise.New),
	}
	h, err := libp2p.New(append(base, opts...)...)
	if err != nil {
		return nil, fmt.Errorf("creating host: %w", err)
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

// Connect
func Connect(ctx context.Context, h host.Host, fullAddr string) (peer.ID, error) {
	return ConnectAddrs(ctx, h, []string{fullAddr})
}

// dials one peer that is reachable at several addresses, same peerID is a must
func ConnectAddrs(ctx context.Context, h host.Host, fullAddrs []string) (peer.ID, error) {
	maddrs := make([]multiaddr.Multiaddr, 0, len(fullAddrs))
	for _, s := range fullAddrs {
		m, err := multiaddr.NewMultiaddr(s)
		if err != nil {
			return "", fmt.Errorf("parsing multiaddr %q: %w", s, err)
		}
		maddrs = append(maddrs, m)
	}

	infos, err := peer.AddrInfosFromP2pAddrs(maddrs...)
	if err != nil {
		return "", fmt.Errorf("extracting peer info: %w", err)
	}
	if len(infos) != 1 {
		return "", fmt.Errorf("addresses must all belong to exactly one peer, found %d", len(infos))
	}

	if err := h.Connect(ctx, infos[0]); err != nil {
		return "", fmt.Errorf("connecting to %s: %w", infos[0].ID, err)
	}
	return infos[0].ID, nil
}
