package provider

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/NethermindEth/juno/core/felt"
)

// TestIntegrationSepoliaRPC connects to a real Starknet RPC and verifies
// provider operations. Skipped unless IBIS_INTEGRATION=1 and IBIS_RPC_URL are set.
func TestIntegrationSepoliaRPC(t *testing.T) {
	if os.Getenv("IBIS_INTEGRATION") != "1" {
		t.Skip("set IBIS_INTEGRATION=1 to run integration tests")
	}

	rpcURL := os.Getenv("IBIS_RPC_URL")
	if rpcURL == "" {
		t.Skip("set IBIS_RPC_URL to run integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	t.Log("Connecting to RPC:", rpcURL)
	t.Log("HTTP URL:", ToHTTPURL(rpcURL))
	t.Log("WS URL:  ", ToWSURL(rpcURL))

	p, err := New(ctx, rpcURL, nil)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer p.Close()
	t.Log("Provider created successfully")

	// 1. Fetch block number
	blockNum, err := p.BlockNumber(ctx)
	if err != nil {
		t.Fatalf("BlockNumber() error: %v", err)
	}
	if blockNum == 0 {
		t.Fatal("BlockNumber() returned 0")
	}
	t.Logf("Latest block number: %d", blockNum)

	// 2. Fetch events from last 5 blocks
	from := blockNum - 5
	events, err := p.GetEvents(ctx, GetEventsOptions{
		FromBlock: from,
		ToBlock:   blockNum,
		ChunkSize: 100,
	})
	if err != nil {
		t.Fatalf("GetEvents() error: %v", err)
	}
	t.Logf("Got %d events in block range [%d, %d]", len(events), from, blockNum)

	for i, e := range events {
		if i >= 3 {
			t.Logf("  ... and %d more", len(events)-3)
			break
		}
		t.Logf("  [%d] block=%d tx=%s keys=%d data=%d",
			i, e.BlockNumber, e.TransactionHash, len(e.Keys), len(e.Data))
	}

	// 3. Fetch contract class (ETH token on Sepolia)
	ethAddress := new(felt.Felt).SetBytes(hexToBytes(t,
		"049d36570d4e46f48e99674bd3fcc84644ddd6b96f7c741b1562b82f9e004dc7"))
	classJSON, err := p.GetClassAt(ctx, ethAddress)
	if err != nil {
		t.Fatalf("GetClassAt() error: %v", err)
	}
	t.Logf("GetClassAt returned %d bytes of ABI JSON", len(classJSON))
	if len(classJSON) < 100 {
		t.Fatal("GetClassAt returned suspiciously small response")
	}
}

func hexToBytes(t *testing.T, hex string) []byte {
	t.Helper()
	b := make([]byte, len(hex)/2)
	for i := 0; i < len(hex); i += 2 {
		_, err := fmt.Sscanf(hex[i:i+2], "%02x", &b[i/2])
		if err != nil {
			t.Fatalf("bad hex at %d: %v", i, err)
		}
	}
	return b
}
