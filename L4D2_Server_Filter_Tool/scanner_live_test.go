package main

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestLiveQueryMaster(t *testing.T) {
	if os.Getenv("L4D2_TOOL_LIVE_TEST") != "1" {
		t.Skip("set L4D2_TOOL_LIVE_TEST=1 to query Steam master servers")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	addrs, err := queryMaster(ctx, 0xFF, 25)
	if err != nil {
		t.Fatal(err)
	}
	if len(addrs) == 0 {
		t.Fatal("empty server list")
	}
	t.Logf("got %d servers, first=%s", len(addrs), addrs[0])
}
