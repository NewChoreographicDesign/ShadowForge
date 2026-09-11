package walletapi

import (
	"context"

	"github.com/shadowforge/shadowforge-l1/pkg/queryclient"
)

// StatusResult is a live node's real chain head, via pkg/query's
// /v1/status endpoint.
type StatusResult struct {
	Height    uint64
	HeadHash  string
	GenesisMs int64
}

// Status fetches queryURL's real, live chain head.
func Status(queryURL string) (StatusResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	st, err := queryclient.New(queryURL).Status(ctx)
	if err != nil {
		return StatusResult{}, err
	}
	return StatusResult{Height: st.Height, HeadHash: st.HeadHash.String(), GenesisMs: st.GenesisMs}, nil
}
