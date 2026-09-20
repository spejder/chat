// Command chat runs the chat web server.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spejder/chat/internal/auth"
	"github.com/spejder/chat/internal/postgres"
	"github.com/spejder/chat/internal/server"
	"github.com/spejder/chat/internal/sms"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	addr := flag.String("addr", defaultAddr(), "address the server listens on")
	databaseURL := flag.String("database-url", os.Getenv("DATABASE_URL"), "address of the PostgreSQL server")
	migrateOnly := flag.Bool("migrate-only", false, "apply the migrations and stop")
	origin := flag.String("origin", defaultOrigin(), "address of this site, which a passkey belongs to")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if *databaseURL == "" {
		return errors.New("no database address, set DATABASE_URL or pass -database-url")
	}

	pool, err := postgres.Open(ctx, *databaseURL)
	if err != nil {
		return err
	}

	defer pool.Close()

	if err := postgres.Migrate(ctx, pool); err != nil {
		return err
	}

	slog.Info("the database is ready")

	if *migrateOnly {
		return nil
	}

	signIn, err := auth.New(
		postgres.NewUserStore(pool),
		postgres.NewAuthStore(pool),
		sms.StdoutSender{},
		*origin,
	)
	if err != nil {
		return err
	}

	slog.Info("the sign in is ready", "origin", *origin)

	srv := &http.Server{
		Addr: *addr,
		Handler: server.New(server.Config{
			Auth:          signIn,
			SecureCookies: strings.HasPrefix(*origin, "https://"),
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errs := make(chan error, 1)

	go func() {
		slog.Info("listening", "addr", srv.Addr)

		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- err
		}
	}()

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
		slog.Info("shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return srv.Shutdown(shutdownCtx)
}

// defaultOrigin reads the ORIGIN environment variable. A passkey belongs to
// one site, so this address must be the address that the browser shows.
func defaultOrigin() string {
	if origin := os.Getenv("ORIGIN"); origin != "" {
		return origin
	}

	return "http://localhost:8080"
}

// defaultAddr reads the PORT environment variable, which some hosts set.
func defaultAddr() string {
	if port := os.Getenv("PORT"); port != "" {
		return ":" + port
	}

	return ":8080"
}
