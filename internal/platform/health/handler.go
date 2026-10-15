package health

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

const DBTimeout = time.Second

type Pinger interface {
	Ping(ctx context.Context) error
}

type Handler struct {
	db      Pinger
	logger  *slog.Logger
	timeout time.Duration
}

func NewHandler(db Pinger, logger *slog.Logger) *Handler {
	return NewHandlerWithTimeout(db, logger, DBTimeout)
}

func NewHandlerWithTimeout(db Pinger, logger *slog.Logger, timeout time.Duration) *Handler {
	return &Handler{db: db, logger: logger, timeout: timeout}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", h.healthz)
}

type response struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

func (h *Handler) healthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		h.logger.WarnContext(r.Context(), "health check failed", "check", "db", "err", err)
		state := "unavailable"
		if errors.Is(err, context.DeadlineExceeded) {
			state = "timeout"
		}
		h.writeJSON(w, http.StatusServiceUnavailable, response{Status: "degraded", Checks: map[string]string{"db": state}})
		return
	}
	h.writeJSON(w, http.StatusOK, response{Status: "ok"})
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		h.logger.Warn("write json failed", "err", err)
	}
}
