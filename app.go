package main

import (
	"Unakite/internal/session"
	"Unakite/internal/signaling"
	"context"
	"os"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const defaultSignalUrl = "http://127.0.0.1:8080"

type App struct {
	ctx     context.Context
	session *session.Session
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	url := os.Getenv("SIGNAL_URL")
	if url == "" {
		url = defaultSignalUrl
	}
	a.session = session.New(signaling.NewClient(url), func(event string, payload any) {
		runtime.EventsEmit(a.ctx, event, payload)
	})
}

func (a *App) shutdown(ctx context.Context) {
	a.session.Leave()
}

func (a *App) CreateRoom(nickname string) (string, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	return a.session.CreateRoom(ctx, nickname)
}

func (a *App) JoinRoom(nickname, code string) error {
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	return a.session.JoinRoom(ctx, nickname, code)
}

func (a *App) SendMessage(content string) (session.Message, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
	defer cancel()
	return a.session.SendMessage(ctx, content)
}

func (a *App) GetState() session.State {
	return a.session.State()
}

func (a *App) LeaveRoom() { a.session.Leave() }
