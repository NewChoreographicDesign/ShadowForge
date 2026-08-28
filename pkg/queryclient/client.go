// Package queryclient is a real Go client for every endpoint pkg/query's
// Server exposes. pkg/txclient already wraps one of them (/v1/tx/{txid})
// for its own submit-and-confirm loop; this package is the rest — status,
// blocks, nullifier/note existence, NFTs, bank holds, and governance
// proposals — so a caller (this session's cmd/wallet included) never has
// to hand-roll an HTTP GET and JSON decode per endpoint. Every method
// here is a thin, honest mirror of what pkg/query's own doc says it
// exposes: nothing more is decoded or exported than the server actually
// sends.
package queryclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/shadowforge/shadowforge-l1/pkg/types"
)

// ErrNotFound is returned by a lookup whose pkg/query endpoint answered
// HTTP 404 — a real "doesn't exist," not a transport failure.
var ErrNotFound = errors.New("queryclient: not found")

// DefaultHTTPTimeout bounds a single request.
const DefaultHTTPTimeout = 5 * time.Second

// Client is a real HTTP client bound to one pkg/query base URL (e.g.
// "http://127.0.0.1:8081", no trailing slash needed).
type Client struct {
	base string
	http *http.Client
}

// New builds a Client with the default HTTP timeout.
func New(baseURL string) *Client {
	return &Client{base: baseURL, http: &http.Client{Timeout: DefaultHTTPTimeout}}
}

// NewWithClient builds a Client using a caller-supplied *http.Client
// (e.g. one with a different timeout, or shared across several Clients).
func NewWithClient(baseURL string, hc *http.Client) *Client {
	return &Client{base: baseURL, http: hc}
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return fmt.Errorf("queryclient: build request for %s: %w", path, err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("queryclient: request %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("queryclient: %s returned HTTP %d: %s", path, resp.StatusCode, body)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("queryclient: decode response from %s: %w", path, err)
	}
	return nil
}

// Status is a node's real, live chain head.
type Status struct {
	Height    uint64
	HeadHash  types.Hash
	GenesisMs int64
}

type statusResponse struct {
	Height    uint64 `json:"height"`
	HeadHash  string `json:"head_hash"`
	GenesisMs int64  `json:"genesis_ms"`
}

// Status fetches /v1/status.
func (c *Client) Status(ctx context.Context) (Status, error) {
	var resp statusResponse
	if err := c.get(ctx, "/v1/status", &resp); err != nil {
		return Status{}, err
	}
	headHash, err := types.ParseHash(resp.HeadHash)
	if err != nil {
		return Status{}, fmt.Errorf("queryclient: parse head_hash: %w", err)
	}
	return Status{Height: resp.Height, HeadHash: headHash, GenesisMs: resp.GenesisMs}, nil
}

// Block fetches the full committed block at height via /v1/blocks/{height}.
// Returns ErrNotFound if no block exists at that height yet.
func (c *Client) Block(ctx context.Context, height uint64) (types.Block, error) {
	var b types.Block
	if err := c.get(ctx, fmt.Sprintf("/v1/blocks/%d", height), &b); err != nil {
		return types.Block{}, err
	}
	return b, nil
}

// TxStatus is one transaction's node-local confirmation state — see
// pkg/query's own doc on why this is a node-local view, not a global
// guarantee.
type TxStatus struct {
	Status string
	Height *uint64
}

type txStatusResponse struct {
	Status string  `json:"status"`
	Height *uint64 `json:"height,omitempty"`
}

// TxStatus fetches /v1/tx/{txid}. Status is one of "committed",
// "pending", or "unknown" — never an error on its own; pkg/query always
// answers 200 here even when it has no idea about the transaction.
func (c *Client) TxStatus(ctx context.Context, txid types.Hash) (TxStatus, error) {
	var resp txStatusResponse
	if err := c.get(ctx, "/v1/tx/"+txid.String(), &resp); err != nil {
		return TxStatus{}, err
	}
	return TxStatus(resp), nil
}

type nullifierResponse struct {
	Spent bool `json:"spent"`
}

// NullifierSpent reports whether a shielded note's nullifier has already
// been spent, via /v1/nullifier/{hash}.
func (c *Client) NullifierSpent(ctx context.Context, nullifier types.Hash) (bool, error) {
	var resp nullifierResponse
	if err := c.get(ctx, "/v1/nullifier/"+nullifier.String(), &resp); err != nil {
		return false, err
	}
	return resp.Spent, nil
}

type noteExistsResponse struct {
	Exists bool `json:"exists"`
}

// NoteExists reports whether a shielded note commitment has been
// committed, via /v1/note/{commitment}. It never reveals the note's
// value, owner, or any other private field — see pkg/query's own doc.
func (c *Client) NoteExists(ctx context.Context, commitment types.Hash) (bool, error) {
	var resp noteExistsResponse
	if err := c.get(ctx, "/v1/note/"+commitment.String(), &resp); err != nil {
		return false, err
	}
	return resp.Exists, nil
}

// NFT fetches a validator NFT record via /v1/nft/{id}. Returns
// ErrNotFound if id was never minted.
func (c *Client) NFT(ctx context.Context, id types.NFTID) (types.ValidatorNFT, error) {
	var nft types.ValidatorNFT
	if err := c.get(ctx, "/v1/nft/"+id.String(), &nft); err != nil {
		return types.ValidatorNFT{}, err
	}
	return nft, nil
}

// Hold fetches a bank hold record via /v1/hold/{id}. Returns ErrNotFound
// if no hold exists with that id.
func (c *Client) Hold(ctx context.Context, id types.Hash) (types.BankHold, error) {
	var hold types.BankHold
	if err := c.get(ctx, "/v1/hold/"+id.String(), &hold); err != nil {
		return types.BankHold{}, err
	}
	return hold, nil
}

// Proposal is a deliberate projection of a governance proposal's real
// aggregate tally — see pkg/query's own doc on why per-voter data is
// never exposed here.
type Proposal struct {
	ProposalID string
	Epoch      uint64
	ParamKey   string
	NewValue   string
	Tallied    bool
	Approve    int
	Reject     int
	Passed     bool
	Applied    bool
}

type proposalResponse struct {
	ProposalID string `json:"proposal_id"`
	Epoch      uint64 `json:"epoch"`
	ParamKey   string `json:"param_key,omitempty"`
	NewValue   string `json:"new_value,omitempty"`
	Tallied    bool   `json:"tallied"`
	Approve    int    `json:"approve"`
	Reject     int    `json:"reject"`
	Passed     bool   `json:"passed"`
	Applied    bool   `json:"applied"`
}

func (p proposalResponse) toProposal() Proposal { return Proposal(p) }

// Proposal fetches one governance proposal's aggregate tally via
// /v1/proposal/{id}. Returns ErrNotFound if no proposal with that id
// exists.
func (c *Client) Proposal(ctx context.Context, id string) (Proposal, error) {
	var resp proposalResponse
	if err := c.get(ctx, "/v1/proposal/"+id, &resp); err != nil {
		return Proposal{}, err
	}
	return resp.toProposal(), nil
}

// Proposals lists every known governance proposal's aggregate tally via
// /v1/proposals.
func (c *Client) Proposals(ctx context.Context) ([]Proposal, error) {
	var resp []proposalResponse
	if err := c.get(ctx, "/v1/proposals", &resp); err != nil {
		return nil, err
	}
	out := make([]Proposal, 0, len(resp))
	for _, p := range resp {
		out = append(out, p.toProposal())
	}
	return out, nil
}
