package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"seesharpsi/web_roguelike/handlers"
	"seesharpsi/web_roguelike/match"
	"seesharpsi/web_roguelike/session"
)

func main() {
	port := flag.Int("port", 9779, "port the server runs on")
	address := flag.String("address", "http://localhost", "address the server runs on")
	flag.Parse()

	// Parse the configured listen address.
	base_ip := *address
	ip := base_ip + ":" + strconv.Itoa(*port)
	root_ip, err := url.Parse(ip)
	if err != nil {
		log.Panic(err)
	}

	sessionManager := session.NewManager()
	matchRegistry := match.NewRegistry()

	h := &handlers.Handler{
		Manager: sessionManager,
		Matches: matchRegistry,
	}

	// Serve static assets separately from dynamic application responses.
	mux := http.NewServeMux()
	fs := http.FileServer(http.Dir("./static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))
	mux.Handle("/", h.Routes())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go cleanupLoop(ctx, sessionManager, matchRegistry)

	server := http.Server{
		Addr:        root_ip.Host,
		Handler:     mux,
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("server shutdown: %v", err)
		}
	}()

	// Serve requests until shutdown.
	log.Printf("running server on %s\n", root_ip.Host)
	defer server.Close()
	err = server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		<-shutdownDone
		log.Printf("server closed\n")
	} else if err != nil {
		log.Printf("error starting server: %s\n", err)
		os.Exit(1)
	}
}

func cleanupLoop(ctx context.Context, sessions *session.Manager, matches *match.Registry) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case now := <-ticker.C:
			sessions.Cleanup(now, 24*time.Hour)
			matches.Cleanup(now, 2*time.Hour)
		case <-ctx.Done():
			return
		}
	}
}
