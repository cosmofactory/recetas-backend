package dish

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /dishes", h.create)
	mux.HandleFunc("GET /dishes", h.list)
	mux.HandleFunc("GET /dishes/{id}", h.get)
	mux.HandleFunc("GET /dishes/total/count", h.countDishes)
}

type createDishRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req createDishRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	d, err := h.svc.CreateDish(r.Context(), req.Name, req.Description)
	if err != nil {
		log.Printf("create dish: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	dishes, err := h.svc.ListDishes(r.Context())
	if err != nil {
		log.Printf("list dishes: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, dishes)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	d, err := h.svc.GetDish(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "dish not found")
			return
		}
		log.Printf("get dish: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *Handler) countDishes(w http.ResponseWriter, r *http.Request) {
	count, err := h.svc.CountDishes(r.Context())
	if err != nil {
		log.Printf("count dishes: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"count": count})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write json: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
