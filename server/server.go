package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/log/v2"
	"charm.land/wish/v2"
	"charm.land/wish/v2/bubbletea"
	"charm.land/wish/v2/logging"
	"github.com/charmbracelet/ssh"
	"github.com/peterjohnbishop/centra-chatter/storage"
	"github.com/peterjohnbishop/centra-chatter/tui"
)

const (
	host = "0.0.0.0"
	port = "23234"
)

func ServeWish(db *storage.Storage) {
	s, err := wish.NewServer(
		wish.WithAddress(fmt.Sprintf("%s:%s", host, port)),
		wish.WithHostKeyPath(".ssh/id_ed25519"),

		wish.WithPublicKeyAuth(func(ctx ssh.Context, key ssh.PublicKey) bool {
			return true
		}),
		wish.WithPasswordAuth(func(ctx ssh.Context, password string) bool {
			return true // Accepts all passwords for guests
		}),

		wish.WithMiddleware(
			logging.Middleware(),
			myCustomApp(db),
		),
	)
	if err != nil {
		log.Fatal("Could not create server", "err", err)
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	log.Info("Starting SSH server", "host", host, "port", port)
	go func() {
		if err := s.ListenAndServe(); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
			log.Fatal("Could not start server", "err", err)
		}
	}()

	<-done
	log.Info("Stopping SSH server")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
		log.Fatal("Could not stop server", "err", err)
	}
}

func myCustomApp(db *storage.Storage) wish.Middleware {
	return bubbletea.Middleware(func(s ssh.Session) (tea.Model, []tea.ProgramOption) {
		username := s.User()
		pubKey := s.PublicKey()

		isAuthenticated := false
		if pubKey != nil {
			isAuthenticated = db.ValidatePublicKey(username, pubKey)
		}

		remote := s.RemoteAddr().String()
		isLocalhost := strings.HasPrefix(remote, "127.0.0.1") || strings.HasPrefix(remote, "[::1]")
		isAdmin := isLocalhost || (isAuthenticated && db.IsAdmin(username))

		m := tui.InitialModel(db, isAdmin, isAuthenticated, pubKey, username)

		return m, nil
	})
}
