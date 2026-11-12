package signaling

import (
	"encoding/json"
	"errors"
	"net/http"
)

// maximum number of bytes from req body
const maxBytes = 16 << 10

type CreateRoomRequest struct {
	Addrs []string `json:"addrs"`
}

type CreateRoomResponse struct {
	Code string `json:"code"`
}

type JoinRoomResponse struct {
	Addrs []string `json:"addrs"`
}

type errorResponse struct {
	Error string `json:"error"`
}

type Server struct {
	store *Store
}

func NewServer(store *Store) *Server {
	return &Server{
		store: store,
	}
}

// returns a http.Handler that serves the signaling server endpoints.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /rooms", s.handleCreate)
	mux.HandleFunc("POST /rooms/{code}/join", s.handleJoin)
	return mux
}

// handles POST /rooms
func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

	var req CreateRoomRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	code, err := s.store.Create(req.Addrs)
	if err != nil {
		if errors.Is(err, ErrInvalidAddrs) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create room")
		return
	}

	writeJSON(w, http.StatusCreated, CreateRoomResponse{Code: code})
}

// handles POST /rooms/{code}/join
func (s *Server) handleJoin(w http.ResponseWriter, r *http.Request) {
	addrs, err := s.store.Consume(r.PathValue("code"))
	if err != nil {
		writeError(w, http.StatusNotFound, "room not found or the code has already been used")
		return
	}
	writeJSON(w, http.StatusOK, JoinRoomResponse{Addrs: addrs})
}

// JSON handling
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// error handling
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}
