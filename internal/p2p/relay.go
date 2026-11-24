package p2p

import (
	"Unakite/internal/signaling"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

var ErrRelayUnavailable = errors.New("relay unavailable")

// returns libp2p options for a host using a specific relay
func HostRelayOptions(relay peer.AddrInfo) []libp2p.Option {
	return []libp2p.Option{
		libp2p.ForceReachabilityPrivate(),
		libp2p.EnableAutoRelayWithStaticRelays([]peer.AddrInfo{relay}),
		libp2p.EnableHolePunching(),
	}
}

// returns libp2p options for a guest host (not using a specific relay)
func GuestRelayOptions() []libp2p.Option {
	return []libp2p.Option{
		libp2p.ForceReachabilityPrivate(),
		libp2p.EnableHolePunching(),
	}
}

// creates a room via the specified relay and returns the room code
func CreateRoomViaRelay(ctx context.Context, sc *signaling.Client, h host.Host, relay peer.AddrInfo) (string, error) {
	if err := waitForReservation(ctx, h); err != nil {
		return "", err
	}
	addrs := circuitAddrs(relay, h.ID())
	if len(addrs) == 0 {
		return "", errors.New("relay has no network address")
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

// waits until the host has a relay reservation or the context is done
func waitForReservation(ctx context.Context, h host.Host) error {
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		for _, a := range h.Addrs() {
			if strings.Contains(a.String(), "/p2p-circuit") {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%w (%v)", ErrRelayUnavailable, ctx.Err())
		case <-tick.C:
		}
	}
}

// returns the circuit addresses for the relay and the host's own ID
func circuitAddrs(relay peer.AddrInfo, self peer.ID) []string {
	out := make([]string, 0, len(relay.Addrs))
	for _, a := range relay.Addrs {
		out = append(out, fmt.Sprintf("%s/p2p/%s/p2p-circuit/p2p/%s", a, relay.ID, self))
	}
	return out
}
