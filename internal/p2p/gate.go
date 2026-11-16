package p2p

import (
	"sync"

	"github.com/libp2p/go-libp2p/core/connmgr"
	"github.com/libp2p/go-libp2p/core/control"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

// connection gater for the room gate
var _ connmgr.ConnectionGater = (*RoomGate)(nil)

type RoomGate struct {
	mu    sync.Mutex
	guest peer.ID
}

func NewRoomGate() *RoomGate {
	return &RoomGate{}
}

// returns the guest peer id
func (g *RoomGate) Guest() peer.ID {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.guest
}

// determines if the peer is allowed to connect to the room
func (g *RoomGate) InterceptSecured(dir network.Direction, p peer.ID, _ network.ConnMultiaddrs) bool {
	if dir == network.DirOutbound {
		return true
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.guest == "" {
		g.guest = p
		return true
	}
	return g.guest == p
}

// determines if the peer is allowed to dial out, always returns true
// determines if the peer is allowed to dial a specific address, always returns true
// determines if the peer is allowed to accept incoming connections, always returns true
// determines if the upgraded connection is allowed, always returns true with no disconnect reason
func (g *RoomGate) InterceptPeerDial(peer.ID) bool                                  { return true }
func (g *RoomGate) InterceptAddrDial(peer.ID, multiaddr.Multiaddr) bool             { return true }
func (g *RoomGate) InterceptAccept(network.ConnMultiaddrs) bool                     { return true }
func (g *RoomGate) InterceptUpgraded(network.Conn) (bool, control.DisconnectReason) { return true, 0 }
