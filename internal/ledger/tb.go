package ledger

import (
	"errors"
	"fmt"
	"hash/fnv"
	"math/big"
	"os"

	tigerbeetle "github.com/tigerbeetle/tigerbeetle-go"
	"github.com/tigerbeetle/tigerbeetle-go/pkg/types"
)

const (
	LedgerNGN         uint32 = 1
	CodeDeposit       uint16 = 1
	treasuryAccountID uint64 = 1
)

type Client struct {
	tb tigerbeetle.Client
}

func NewClient() (*Client, error) {
	addr := os.Getenv("TIGERBEETLE_ADDR")
	if addr == "" {
		addr = "3001"
	}

	tb, err := tigerbeetle.NewClient(types.ToUint128(0), []string{addr})
	if err != nil {
		return nil, fmt.Errorf("ledger: connect: %w", err)
	}

	c := &Client{tb: tb}
	if err := c.ensureTreasury(); err != nil {
		return nil, err
	}
	return c, nil
}

// EnsureAccount creates a TigerBeetle account for an address if it doesn't exist.
func (c *Client) EnsureAccount(address string) error {
	id := types.ToUint128(addressToID(address))
	results, err := c.tb.CreateAccounts([]types.Account{{
		ID:     id,
		Ledger: LedgerNGN,
		Code:   1,
		Flags:  types.AccountFlags{DebitsMustNotExceedCredits: true}.ToUint16(),
	}})
	if err != nil {
		return fmt.Errorf("ledger: create account: %w", err)
	}
	for _, r := range results {
		if r.Result != types.AccountExists {
			return fmt.Errorf("ledger: create account error: %v", r.Result)
		}
	}
	return nil
}

// CreditDeposit records an NGN credit to the address's account.
// amountKobo is the NGN amount in kobo (1 NGN = 100 kobo).
// txHash is used as the transfer ID for idempotency.
func (c *Client) CreditDeposit(address string, amountKobo uint64, txHash string) error {
	results, err := c.tb.CreateTransfers([]types.Transfer{{
		ID:              hashToUint128(txHash),
		DebitAccountID:  types.ToUint128(treasuryAccountID),
		CreditAccountID: types.ToUint128(addressToID(address)),
		Amount:          types.ToUint128(amountKobo),
		Ledger:          LedgerNGN,
		Code:            CodeDeposit,
	}})
	if err != nil {
		return fmt.Errorf("ledger: credit deposit: %w", err)
	}
	for _, r := range results {
		if r.Result != types.TransferExists {
			return fmt.Errorf("ledger: transfer error: %v", r.Result)
		}
	}
	return nil
}

// Balance returns the NGN balance in kobo for an address.
func (c *Client) Balance(address string) (uint64, error) {
	id := types.ToUint128(addressToID(address))
	accounts, err := c.tb.LookupAccounts([]types.Uint128{id})
	if err != nil {
		return 0, fmt.Errorf("ledger: lookup: %w", err)
	}
	if len(accounts) == 0 {
		return 0, errors.New("ledger: account not found")
	}
	a := accounts[0]
	credits := a.CreditsPosted.BigInt()
	debits := a.DebitsPosted.BigInt()
	balance := new(big.Int).Sub(&credits, &debits)
	if !balance.IsUint64() {
		return 0, errors.New("ledger: balance overflow")
	}
	return balance.Uint64(), nil
}

func (c *Client) Close() {
	c.tb.Close()
}

func (c *Client) ensureTreasury() error {
	results, err := c.tb.CreateAccounts([]types.Account{{
		ID:     types.ToUint128(treasuryAccountID),
		Ledger: LedgerNGN,
		Code:   1,
		Flags:  0,
	}})
	if err != nil {
		return fmt.Errorf("ledger: ensure treasury: %w", err)
	}
	for _, r := range results {
		if r.Result != types.AccountExists {
			return fmt.Errorf("ledger: treasury account error: %v", r.Result)
		}
	}
	return nil
}

// addressToID maps a wallet address to a stable uint64 for TigerBeetle.
// FNV-64a output is far from 1 (treasury), so no collision risk in practice.
func addressToID(address string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(address))
	return h.Sum64()
}

// hashToUint128 derives a stable transfer ID from a tx hash.
func hashToUint128(s string) types.Uint128 {
	b := []byte(s)
	if len(b) < 16 {
		padded := make([]byte, 16)
		copy(padded, b)
		b = padded
	}
	var arr [16]byte
	copy(arr[:], b[:16])
	return types.BytesToUint128(arr)
}
