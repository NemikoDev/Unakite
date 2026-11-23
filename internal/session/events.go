package session

// defines event types, payload structures, and the emitter function for session events.

const (
	EventMessage    = "chat:message"
	EventPeerJoined = "room:peer-joined"
	EventPeerLeft   = "room:peer-left"
)

const EventRoomClosed = "room:closed"

// function type for emitting events to listeners
type Emitter func(event string, payload any)

type PeerInfo struct {
	ID       string `json:"id"`
	Nickname string `json:"nickname"`
}

type PeerLeft struct {
	PeerInfo
	ClosesAt int64 `json:"closesAt"`
}

type Message struct {
	ID        string   `json:"id"`
	Sender    PeerInfo `json:"sender"`
	Content   string   `json:"content"`
	Timestamp int64    `json:"timestamp"`
	IsOwn     bool     `json:"isOwn"`
}

type RoomClosed struct {
	Reason string `json:"reason"`
}

type State struct {
	InRoom   bool      `json:"inRoom"`
	Code     string    `json:"code"`
	Self     PeerInfo  `json:"self"`
	Peer     *PeerInfo `json:"peer"`
	ClosesAt int64     `json:"closesAt"`
}
