package p2p

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
)

// protocol ID for the chat service
const ChatProtocolID protocol.ID = "/poc-chat/1.0.0"

// JSON payload sent over stream
type ChatMessage struct {
	Nickname  string `json:"nickname"`
	Content   string `json:"content"`
	Timestamp int64  `json:"timestamp"`
}

// callback function for icoming messages
type MessageHandler func(from peer.ID, msg ChatMessage)

// outbound stream
type outbound struct {
	stream network.Stream
	enc    *json.Encoder
}

// chat service over p2p streams. Sends + receives
type ChatService struct {
	host      host.Host
	onMessage MessageHandler
	mu        sync.Mutex
	out       map[peer.ID]*outbound
}

// NewChatService creates a new chat service for the given host and message handler.
func NewChatService(h host.Host, onMessage MessageHandler) *ChatService {
	c := &ChatService{
		host:      h,
		onMessage: onMessage,
		out:       make(map[peer.ID]*outbound),
	}
	h.SetStreamHandler(ChatProtocolID, c.handleStream)
	return c
}

// Close the chat service and clean up resources.
func (c *ChatService) Close() {
	c.host.RemoveStreamHandler(ChatProtocolID)
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, o := range c.out {
		o.stream.Close()
		delete(c.out, id)
	}
}

// handleStream handles an incoming stream from a peer.
func (c *ChatService) handleStream(s network.Stream) {
	defer s.Close()
	from := s.Conn().RemotePeer()
	dec := json.NewDecoder(s)
	for {
		var msg ChatMessage
		if err := dec.Decode(&msg); err != nil {
			if !errors.Is(err, io.EOF) {
				s.Reset()
			}
		}
		if c.onMessage != nil {
			c.onMessage(from, msg)
		}
	}
}

// Send a chat message to a peer.
func (c *ChatService) Send(ctx context.Context, to peer.ID, msg ChatMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	o, ok := c.out[to]
	if !ok {
		s, err := c.host.NewStream(ctx, to, ChatProtocolID)
		if err != nil {
			return fmt.Errorf("opening chat stream to %s: %w", to, err)
		}
		o = &outbound{
			stream: s,
			enc:    json.NewEncoder(s),
		}
		c.out[to] = o
	}
	if err := o.enc.Encode(msg); err != nil {
		o.stream.Close()
		delete(c.out, to)
		return fmt.Errorf("sending chat message to %s: %w", to, err)
	}
	return nil
}
