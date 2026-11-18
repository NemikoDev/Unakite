package session

const (
	EventMessage    = "chat:message"
	EventPeerJoined = "room:peer-joined"
	EventPeerLeft   = "room:peer-left"
)

// function type for emitting events to listeners
type Emitter func(event string, payload any)

type PeerInfo struct {
	ID       string `json:"id"`
	Nickname string `json:"nickname"`
}

type Message struct {
	ID        string   `json:"id"`
	Sender    PeerInfo `json:"sender"`
	Content   string   `json:"content"`
	Timestamp int64    `json:"timestamp"`
	IsOwn     bool     `json:"isOwn"`
}
