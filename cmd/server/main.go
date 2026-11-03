package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"coding_agent_management/internal/config"
	"coding_agent_management/internal/httpserver"

	_ "modernc.org/sqlite"
)

const shutdownTimeout = 10 * time.Second

func main() {
	if err := run(); err != nil {
		slog.Error("service stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	configuration, err := config.Load()
	if err != nil {
		return err
	}
	slog.Info("service starting", "region", configuration.Region)

	database, err := sql.Open("sqlite", configuration.DatabasePath)
	if err != nil {
		return errors.New("database startup failed")
	}
	defer func() {
		if err := database.Close(); err != nil {
			slog.Error("database shutdown failed", "error", err)
		}
	}()

	pingContext, cancelPing := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancelPing()
	if err := database.PingContext(pingContext); err != nil {
		return errors.New("database startup failed")
	}

	servers := []*http.Server{
		{
			Addr:              configuration.AppAddress,
			Handler:           httpserver.NewApp(configuration.Version),
			ReadHeaderTimeout: shutdownTimeout,
		},
		{
			Addr:              configuration.MetricsAddress,
			Handler:           httpserver.MetricsHandler(),
			ReadHeaderTimeout: shutdownTimeout,
		},
		{
			Addr:              configuration.PprofAddress,
			Handler:           httpserver.PprofHandler(),
			ReadHeaderTimeout: shutdownTimeout,
		},
	}

	serverErrors := make(chan error, len(servers))
	for _, server := range servers {
		go serve(server, serverErrors)
	}

	shutdownSignal, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case <-shutdownSignal.Done():
	case <-serverErrors:
		return errors.New("http server failed")
	}

	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancelShutdown()
	for _, server := range servers {
		if err := server.Shutdown(shutdownContext); err != nil {
			return errors.New("http server shutdown failed")
		}
	}

	return nil
}

func serve(server *http.Server, serverErrors chan<- error) {
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		serverErrors <- err
	}
}
