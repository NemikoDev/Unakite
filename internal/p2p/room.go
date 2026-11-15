package p2p

import (
	"Unakite/internal/signaling"
	"context"
	"fmt"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

func CreateRoom(ctx context.Context, sc *signaling.Client, h host.Host) (string, error) {
	addrs, err := FullAddrs(h)
	if err != nil {
		return "", err
	}
	if len(addrs) > signaling.MaxAddrs {
		addrs = addrs[:signaling.MaxAddrs]
	}

	code, err := sc.CreateRoom(ctx, addrs)
	if err != nil {
		return "", fmt.Errorf("creating room: %w", err)
	}
	return code, nil
}

func JoinRoom(ctx context.Context, sc *signaling.Client, h host.Host, code string) (peer.ID, error) {
	addrs, err := sc.JoinRoom(ctx, code)
	if err != nil {
		return "", fmt.Errorf("joining room: %w", err)
	}
	return ConnectAddrs(ctx, h, addrs)
}
