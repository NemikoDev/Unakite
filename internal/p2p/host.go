package p2p

import (
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
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
