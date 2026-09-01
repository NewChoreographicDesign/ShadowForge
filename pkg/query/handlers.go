package query

import (
	"net/http"
	"strconv"

	"github.com/shadowforge/shadowforge-l1/pkg/state"
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
	// MintAmount/MintOutCommit/MintApplied are a real spec-17.4 epoch
	// mint's bound claim and execution status — see types.
	// VotePublicInputs.MintAmount's own doc. MintOutCommit is safe to
	// expose here: it's already public the moment the binding TxVote
	// itself is gossiped, long before any tally, so this read-only
	// aggregate endpoint isn't a new leak.
	MintAmount    uint64 `json:"mint_amount,omitempty"`
	MintOutCommit string `json:"mint_out_commit,omitempty"`
	MintApplied   bool   `json:"mint_applied"`
	// MintStaked/StakePositionCommit are the staked-yield proposer path's
	// own counterpart of MintOutCommit — see types.VotePublicInputs.
	// MintStaked's own doc. StakePositionCommit is safe to expose for the
	// identical reason MintOutCommit already is: it's already public the
	// moment the binding TxVote is gossiped. pkg/stakewallet's real sync
	// depends on this field to rebuild its local stake-tree mirror.
	MintStaked          bool   `json:"mint_staked,omitempty"`
	StakePositionCommit string `json:"stake_position_commit,omitempty"`
	// SlashTargetNFT/SlashBurn/SlashApplied are a real spec-10.3 slash
	// proposal's bound claim and execution status — see types.
	// VotePublicInputs.SlashTargetNFT's own doc. SlashTargetNFT is safe
	// to expose here for the identical reason MintOutCommit already is.
	SlashTargetNFT string `json:"slash_target_nft,omitempty"`
	SlashBurn      bool   `json:"slash_burn,omitempty"`
	SlashApplied   bool   `json:"slash_applied,omitempty"`
	// UnlockTransferTarget/UnlockTransferApplied are a real spec-10.1
	// transfer-unlock proposal's bound claim and execution status — see
	// types.VotePublicInputs.UnlockTransferTarget's own doc.
	UnlockTransferTarget  string `json:"unlock_transfer_target,omitempty"`
	UnlockTransferApplied bool   `json:"unlock_transfer_applied,omitempty"`
	// ContainerAssetTarget/ContainerAssetApplied are a real spec-11/19.3
	// Bank-asset-authorization proposal's bound claim and execution
	// status — see types.VotePublicInputs.ContainerAssetTarget's own
	// doc. Safe to expose here for the identical reason SlashTargetNFT
	// already is.
	ContainerAssetTarget  string `json:"container_asset_target,omitempty"`
	ContainerAssetApplied bool   `json:"container_asset_applied,omitempty"`
	// UnwindDualSign/UnwindDualSignApplied are a real spec-8.5 dual-sign-
	// retirement proposal's bound claim and execution status — see
	// types.VotePublicInputs.UnwindDualSign's own doc.
	UnwindDualSign        bool `json:"unwind_dual_sign,omitempty"`
	UnwindDualSignApplied bool `json:"unwind_dual_sign_applied,omitempty"`
}

func slashTargetJSON(p state.ProposalRecord) string {
	if p.SlashTargetNFT.IsZero() {
		return ""
	}
	return p.SlashTargetNFT.String()
}

func unlockTransferTargetJSON(p state.ProposalRecord) string {
	if p.UnlockTransferTarget.IsZero() {
		return ""
	}
	return p.UnlockTransferTarget.String()
}

func mintOutCommitJSON(p state.ProposalRecord) string {
	if p.MintAmount == 0 || p.MintStaked {
		return ""
	}
	return p.MintOutCommit.String()
}

func stakePositionCommitJSON(p state.ProposalRecord) string {
	if p.MintAmount == 0 || !p.MintStaked {
		return ""
	}
	return p.StakePositionCommit.String()
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
		ProposalID:            p.ProposalID,
		Epoch:                 p.Epoch,
		ParamKey:              p.ParamKey,
		NewValue:              p.NewValue,
		Tallied:               p.Tallied,
		Approve:               p.Approve,
		Reject:                p.Reject,
		Passed:                p.Passed,
		Applied:               p.Applied,
		MintAmount:            p.MintAmount,
		MintOutCommit:         mintOutCommitJSON(p),
		MintApplied:           p.MintApplied,
		MintStaked:            p.MintStaked,
		StakePositionCommit:   stakePositionCommitJSON(p),
		SlashTargetNFT:        slashTargetJSON(p),
		SlashBurn:             p.SlashBurn,
		SlashApplied:          p.SlashApplied,
		UnlockTransferTarget:  unlockTransferTargetJSON(p),
		UnlockTransferApplied: p.UnlockTransferApplied,
		ContainerAssetTarget:  string(p.ContainerAssetTarget),
		ContainerAssetApplied: p.ContainerAssetApplied,
		UnwindDualSign:        p.UnwindDualSign,
		UnwindDualSignApplied: p.UnwindDualSignApplied,
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
			ProposalID:            p.ProposalID,
			Epoch:                 p.Epoch,
			ParamKey:              p.ParamKey,
			NewValue:              p.NewValue,
			Tallied:               p.Tallied,
			Approve:               p.Approve,
			Reject:                p.Reject,
			Passed:                p.Passed,
			Applied:               p.Applied,
			MintAmount:            p.MintAmount,
			MintOutCommit:         mintOutCommitJSON(p),
			MintApplied:           p.MintApplied,
			MintStaked:            p.MintStaked,
			StakePositionCommit:   stakePositionCommitJSON(p),
			SlashTargetNFT:        slashTargetJSON(p),
			SlashBurn:             p.SlashBurn,
			SlashApplied:          p.SlashApplied,
			UnlockTransferTarget:  unlockTransferTargetJSON(p),
			UnlockTransferApplied: p.UnlockTransferApplied,
			ContainerAssetTarget:  string(p.ContainerAssetTarget),
			ContainerAssetApplied: p.ContainerAssetApplied,
			UnwindDualSign:        p.UnwindDualSign,
			UnwindDualSignApplied: p.UnwindDualSignApplied,
		})
	}
	writeJSON(w, http.StatusOK, out)
}
