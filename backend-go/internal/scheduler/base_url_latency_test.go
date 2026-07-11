package scheduler

import (
	"net"
	"testing"

	"github.com/BenedictKing/ccx/internal/config"
	"github.com/BenedictKing/ccx/internal/warmup"
)

func TestProbeBaseURLTCP(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr == nil {
			_ = conn.Close()
		}
	}()

	result := probeBaseURLTCP("http://" + listener.Addr().String())
	if !result.OK || result.LatencyMs < 0 {
		t.Fatalf("probe result = %#v, want successful TCP connection", result)
	}
	if invalid := probeBaseURLTCP("not a URL"); invalid.OK || invalid.Error != "invalid_url" {
		t.Fatalf("invalid URL probe = %#v", invalid)
	}
}

func TestGetBaseURLLatencySnapshot(t *testing.T) {
	cfg := config.Config{Upstream: []config.UpstreamConfig{{
		Name:     "multi-url",
		Status:   "active",
		BaseURLs: []string{"https://slow.example.com", "https://fast.example.com"},
	}}}
	scheduler, cleanup := createTestScheduler(t, cfg)
	defer cleanup()

	scheduler.ApplyBaseURLProbeResults(ChannelKindMessages, 0, cfg.Upstream[0].BaseURLs, []warmup.URLProbeResult{
		{URL: "https://slow.example.com", OK: true, LatencyMs: 200},
		{URL: "https://fast.example.com", OK: true, LatencyMs: 20},
	})

	snapshot := scheduler.GetBaseURLLatencySnapshot()
	if len(snapshot.Channels) != 1 || len(snapshot.Channels[0].URLs) != 2 {
		t.Fatalf("snapshot = %#v, want one channel with two URLs", snapshot)
	}
	if got := snapshot.Channels[0].URLs[0]; got.URL != "https://fast.example.com" || !got.Preferred {
		t.Fatalf("first URL = %#v, want fast preferred URL", got)
	}
}
