package query

import (
	"net/http"
	"strconv"
)

// statusResponse answers /v1/status.
type statusResponse struct {
	Height    uint64 `json:"height"`
	HeadHash  string `json:"head_hash"`
	GenesisMs int64  `json:"genesis_ms"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, statusResponse{
		Height:    s.chn.HeadHeight(),
		HeadHash:  s.chn.HeadHash().String(),
		GenesisMs: s.genesis,
	})
}

// handleBlock answers /v1/blocks/{height} with the full committed block —
// safe to return in full: every field (including its transaction batch)
// is data real BFT consensus already broadcasts to every peer as part of
// proposing and announcing it, so a query endpoint reveals nothing a
// participant on the network didn't already see live.
func (s *Server) handleBlock(w http.ResponseWriter, r *http.Request) {
	raw := r.PathValue("height")
	height, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "height must be a non-negative integer")
		return
	}
	b, found, err := s.store.GetBlock(height)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		s.logf("query: get block %d: %v", height, err)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "no block at that height")
		return
	}
	writeJSON(w, http.StatusOK, b)
}

// txStatusResponse answers /v1/tx/{txid}. Status is one of "committed",
// "pending", or "unknown" — a node-local view, not a global guarantee: a
// transaction another node's mempool holds but this node hasn't seen yet,
// or one this node's mempool already evicted before it was committed
// anywhere, both honestly read back as "unknown" rather than a false
// negative dressed up as certainty.
type txStatusResponse struct {
	Status string  `json:"status"`
	Height *uint64 `json:"height,omitempty"`
}

func (s *Server) handleTx(w http.ResponseWriter, r *http.Request) {
	txid, err := parseHash(r.PathValue("txid"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "txid must be 32 bytes of hex")
		return
	}

	height, found, err := s.store.GetTxHeight(txid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		s.logf("query: get tx height %s: %v", txid, err)
		return
	}
	if found {
		writeJSON(w, http.StatusOK, txStatusResponse{Status: "committed", Height: &height})
		return
	}

	if s.mempool.Contains(txid) {
		writeJSON(w, http.StatusOK, txStatusResponse{Status: "pending"})
		return
	}

	writeJSON(w, http.StatusOK, txStatusResponse{Status: "unknown"})
}

// nullifierResponse answers /v1/nullifier/{hash} — whether a specific
// note has already been spent. This is the honest, privacy-preserving way
// to check "did my spend land" for a shielded note: it reveals only a
// boolean about a value the caller must already know (the nullifier is
// derived from the note's own secret), never which note or whose it is.
type nullifierResponse struct {
	Spent bool `json:"spent"`
}

func (s *Server) handleNullifier(w http.ResponseWriter, r *http.Request) {
	n, err := parseHash(r.PathValue("hash"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "hash must be 32 bytes of hex")
		return
	}
	spent, err := s.store.IsNullifierSpent(n)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		s.logf("query: is nullifier spent %s: %v", n, err)
		return
	}
	writeJSON(w, http.StatusOK, nullifierResponse{Spent: spent})
}

// noteExistsResponse answers /v1/note/{commitment}. Deliberately the only
// field: GetNote decrypts a note's real Value/OwnerPk/Rho for the
// pipeline's own internal use, and none of that ever reaches this
// handler's response — see this package's doc comment for why that
// boundary matters.
type noteExistsResponse struct {
	Exists bool `json:"exists"`
}

func (s *Server) handleNote(w http.ResponseWriter, r *http.Request) {
	c, err := parseHash(r.PathValue("commitment"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "commitment must be 32 bytes of hex")
		return
	}
	_, found, err := s.store.GetNote(c)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		s.logf("query: get note %s: %v", c, err)
		return
	}
	// found is the only part of GetNote's result that reaches the
	// response — see noteExistsResponse's doc.
	writeJSON(w, http.StatusOK, noteExistsResponse{Exists: found})
}

func (s *Server) handleNFT(w http.ResponseWriter, r *http.Request) {
	id, err := parseNFTID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be 32 bytes of hex")
		return
	}
	nft, found, err := s.store.GetNFT(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		s.logf("query: get nft %s: %v", id, err)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "no NFT with that id")
		return
	}
	writeJSON(w, http.StatusOK, nft)
}

func (s *Server) handleHold(w http.ResponseWriter, r *http.Request) {
	id, err := parseHash(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "id must be 32 bytes of hex")
		return
	}
	hold, found, err := s.store.GetHold(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		s.logf("query: get hold %s: %v", id, err)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "no hold with that id")
		return
	}
	writeJSON(w, http.StatusOK, hold)
}

// proposalResponse is a deliberate projection of state.ProposalRecord,
// not a re-marshal of it: it has no field that could ever carry
// Commitments or Reveals, so a future field added to ProposalRecord can't
// silently start leaking through this endpoint the way reusing that type
// directly could.
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

func (s *Server) handleProposal(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id must not be empty")
		return
	}
	p, found, err := s.store.GetProposal(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		s.logf("query: get proposal %s: %v", id, err)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "no proposal with that id")
		return
	}
	writeJSON(w, http.StatusOK, proposalResponse{
		ProposalID: p.ProposalID,
		Epoch:      p.Epoch,
		ParamKey:   p.ParamKey,
		NewValue:   p.NewValue,
		Tallied:    p.Tallied,
		Approve:    p.Approve,
		Reject:     p.Reject,
		Passed:     p.Passed,
		Applied:    p.Applied,
	})
}

func (s *Server) handleProposals(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListProposals()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		s.logf("query: list proposals: %v", err)
		return
	}
	out := make([]proposalResponse, 0, len(list))
	for _, p := range list {
		out = append(out, proposalResponse{
			ProposalID: p.ProposalID,
			Epoch:      p.Epoch,
			ParamKey:   p.ParamKey,
			NewValue:   p.NewValue,
			Tallied:    p.Tallied,
			Approve:    p.Approve,
			Reject:     p.Reject,
			Passed:     p.Passed,
			Applied:    p.Applied,
		})
	}
	writeJSON(w, http.StatusOK, out)
}
