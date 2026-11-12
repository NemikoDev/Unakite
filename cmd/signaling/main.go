package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"Unakite/internal/signaling"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "address to listen on")
	ttl := flag.Duration("ttl", 10*time.Minute, "how long an unused room code stays valid")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store := signaling.NewStore(*ttl)
	go store.RunCleanup(ctx, time.Minute)

	srv := &http.Server{
		Addr:              *addr,
		Handler:           signaling.NewServer(store).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	log.Printf("signaling server listening on %s (code TTL %s)", *addr, *ttl)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
