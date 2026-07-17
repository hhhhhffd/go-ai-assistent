package discord

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

type HTTPServer struct {
	server  *http.Server
	handler *Handler
	logger  *slog.Logger
}

func NewHTTPServer(addr string, handler *Handler, logger *slog.Logger) *HTTPServer {
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	return &HTTPServer{
		handler: handler,
		server: &http.Server{
			Addr:              addr,
			Handler:           mux,
			ReadTimeout:       10 * time.Second,
			ReadHeaderTimeout: 5 * time.Second,
			WriteTimeout:      15 * time.Second,
			IdleTimeout:       60 * time.Second,
		},
		logger: logger,
	}
}

func (s *HTTPServer) Start() error {
	s.logger.Info("discord http server started", "addr", s.server.Addr)
	err := s.server.ListenAndServe()
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return fmt.Errorf("discord http server failed: %w", err)
}

func (s *HTTPServer) Shutdown(ctx context.Context) error {
	s.logger.Info("discord http server shutting down")
	return s.server.Shutdown(ctx)
}

// Wait waits for interaction jobs that were accepted before shutdown.
func (s *HTTPServer) Wait() {
	s.handler.Wait()
}
