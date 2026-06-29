package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/linq/mvp/internal/db"
	"github.com/linq/mvp/internal/ledger"
	"github.com/linq/mvp/internal/webhook"
)

type Handler struct {
	db *db.DB
	tb *ledger.Client
	wh *webhook.Handler
}

func NewHandler(database *db.DB, tb *ledger.Client) *Handler {
	return &Handler{
		db: database,
		tb: tb,
		wh: webhook.NewHandler(database, tb),
	}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /wallet/register", h.registerWallet)
	mux.HandleFunc("GET /balance/{address}", h.getBalance)
	mux.HandleFunc("POST /webhook/deposit", h.wh.ServeHTTP)
	mux.HandleFunc("GET /health", h.health)
	return mux
}

type registerRequest struct {
	Address     string `json:"address"`
	NetworkType string `json:"network_type"` // must match network_id in indexer config e.g. "sui_mainnet"
}

type registerResponse struct {
	Address     string `json:"address"`
	NetworkType string `json:"network_type"`
}

func (h *Handler) registerWallet(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	req.Address = strings.TrimSpace(req.Address)
	if req.Address == "" || req.NetworkType == "" {
		http.Error(w, "address and network_type are required", http.StatusBadRequest)
		return
	}

	if err := h.db.RegisterAddress(r.Context(), req.Address, req.NetworkType); err != nil {
		http.Error(w, fmt.Sprintf("register failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Pre-create the TigerBeetle account so the first deposit doesn't race.
	if err := h.tb.EnsureAccount(req.Address); err != nil {
		http.Error(w, fmt.Sprintf("ledger account failed: %v", err), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, registerResponse{
		Address:     req.Address,
		NetworkType: req.NetworkType,
	})
}

type balanceResponse struct {
	Address  string `json:"address"`
	NGNKobo  uint64 `json:"ngn_kobo"`
	NGNNaira string `json:"ngn_naira"`
}

func (h *Handler) getBalance(w http.ResponseWriter, r *http.Request) {
	address := strings.TrimSpace(r.PathValue("address"))
	if address == "" {
		http.Error(w, "address required", http.StatusBadRequest)
		return
	}

	wallet, err := h.db.LookupAddress(r.Context(), address)
	if err != nil || wallet == nil {
		http.Error(w, "address not registered", http.StatusNotFound)
		return
	}

	kobo, err := h.tb.Balance(address)
	if err != nil {
		http.Error(w, fmt.Sprintf("balance lookup failed: %v", err), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, balanceResponse{
		Address:  address,
		NGNKobo:  kobo,
		NGNNaira: fmt.Sprintf("₦%d.%02d", kobo/100, kobo%100),
	})
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
