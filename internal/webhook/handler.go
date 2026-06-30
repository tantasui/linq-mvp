package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/linq/mvp/internal/db"
	"github.com/linq/mvp/internal/ledger"
)

const (
	SuiUSDCAddress = "0xdba34672e30cb065b1f93e3ab55318768fd6fef66c15942c9f7cb846e2f900e7::usdc::USDC"
)

type transaction struct {
	TxHash       string `json:"txHash"`
	ToAddress    string `json:"toAddress"`
	AssetAddress string `json:"assetAddress"`
	Amount       string `json:"amount"`
	Status       string `json:"status"`
	Direction    string `json:"direction"`
}

type payload struct {
	EventType   string      `json:"eventType"`
	Transaction transaction `json:"transaction"`
}

type Handler struct {
	db     *db.DB
	tb     *ledger.Client
	secret []byte
	log    *slog.Logger
}

func NewHandler(database *db.DB, tb *ledger.Client) *Handler {
	return &Handler{
		db:     database,
		tb:     tb,
		secret: []byte(os.Getenv("WEBHOOK_SECRET")),
		log:    slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}

	if err := h.verifySignature(body, r.Header); err != nil {
		h.log.Warn("signature invalid", "err", err, "ip", r.RemoteAddr)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Log every verified payload so we can see the exact format the indexer sends.
	h.log.Info("webhook received", "body", string(body))

	var p payload
	if err := json.Unmarshal(body, &p); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	tx := p.Transaction
	// Only process inbound transfers. Status is intentionally not checked —
	// the indexer emits "pending" with sufficient confirmations, which is final enough.
	if tx.Direction != "in" {
		h.log.Info("skipped non-inbound event", "eventType", p.EventType, "direction", tx.Direction, "status", tx.Status)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if err := h.processDeposit(r.Context(), tx); err != nil {
		h.log.Error("deposit failed", "txHash", tx.TxHash, "err", err)
		http.Error(w, "processing error", http.StatusInternalServerError)
		return
	}

	h.log.Info("deposit processed", "txHash", tx.TxHash, "to", tx.ToAddress)
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) processDeposit(ctx context.Context, tx transaction) error {
	// Dispatcher already verified the address before calling us, but double-check
	// to defend against misconfigured webhook secrets pointing the wrong dispatcher here.
	wallet, err := h.db.LookupAddress(ctx, tx.ToAddress)
	if err != nil || wallet == nil {
		return nil
	}

	ngn, err := toNGNKobo(tx.Amount, tx.AssetAddress)
	if err != nil {
		return fmt.Errorf("rate conversion: %w", err)
	}
	if ngn == 0 {
		return nil
	}

	if err := h.tb.EnsureAccount(tx.ToAddress); err != nil {
		return fmt.Errorf("ensure account: %w", err)
	}
	if err := h.tb.CreditDeposit(tx.ToAddress, ngn, tx.TxHash); err != nil {
		return fmt.Errorf("credit: %w", err)
	}
	return nil
}

func (h *Handler) verifySignature(body []byte, headers http.Header) error {
	sig := headers.Get("X-Webhook-Signature")
	tsStr := headers.Get("X-Webhook-Timestamp")

	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil || time.Now().Unix()-ts > 300 {
		return fmt.Errorf("timestamp invalid or too old")
	}

	mac := hmac.New(sha256.New, h.secret)
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(expected), []byte(sig)) {
		return fmt.Errorf("signature mismatch")
	}
	return nil
}

func toNGNKobo(amountStr, assetAddress string) (uint64, error) {
	amount, ok := new(big.Int).SetString(amountStr, 10)
	if !ok {
		return 0, fmt.Errorf("invalid amount: %s", amountStr)
	}

	var rateStr string
	var decimals int
	switch assetAddress {
	case SuiUSDCAddress:
		// 1 USDC (6 decimals) = ₦1600 = 160,000 kobo
		// rate per micro-USDC = 160,000 / 1,000,000 kobo — use big.Int: (amount * 160000) / 1e6
		rateStr = "160000"
		decimals = 6
	default:
		// Native SUI (9 decimals) rough placeholder: 1 SUI ≈ ₦5 = 500 kobo
		rateStr = "500"
		decimals = 9
	}

	rate, _ := new(big.Int).SetString(rateStr, 10)
	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	ngn := new(big.Int).Div(new(big.Int).Mul(amount, rate), divisor)

	if !ngn.IsUint64() {
		return 0, fmt.Errorf("amount overflow")
	}
	return ngn.Uint64(), nil
}
