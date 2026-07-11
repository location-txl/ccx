package warmup

import "testing"

func TestApplyProbeResults_PrefersLowestReachableLatency(t *testing.T) {
	manager := NewURLManager(0, 3)
	urls := []string{"https://slow.example.com", "https://fast.example.com", "https://offline.example.com"}
	manager.ApplyProbeResults(1, urls, []URLProbeResult{
		{URL: urls[0], OK: true, LatencyMs: 240},
		{URL: urls[1], OK: true, LatencyMs: 35},
		{URL: urls[2], Error: "timeout"},
	})

	ordered := manager.GetSortedURLs(1, urls)
	if ordered[0].URL != urls[1] || ordered[1].URL != urls[0] || ordered[2].URL != urls[2] {
		t.Fatalf("unexpected URL order: %#v", ordered)
	}

	snapshot := manager.GetProbeSnapshot(1)
	if !snapshot[0].Preferred || snapshot[0].LatencyMs == nil || *snapshot[0].LatencyMs != 35 {
		t.Fatalf("preferred snapshot = %#v, want fast URL", snapshot[0])
	}
}

func TestApplyProbeResults_AllFailedKeepsExistingPreference(t *testing.T) {
	manager := NewURLManager(0, 3)
	urls := []string{"https://slow.example.com", "https://fast.example.com"}
	manager.ApplyProbeResults(1, urls, []URLProbeResult{
		{URL: urls[0], OK: true, LatencyMs: 240},
		{URL: urls[1], OK: true, LatencyMs: 35},
	})
	manager.ApplyProbeResults(1, urls, []URLProbeResult{
		{URL: urls[0], Error: "timeout"},
		{URL: urls[1], Error: "timeout"},
	})

	ordered := manager.GetSortedURLs(1, urls)
	if ordered[0].URL != urls[1] {
		t.Fatalf("all failed probe changed preferred URL to %q, want %q", ordered[0].URL, urls[1])
	}
	if snapshot := manager.GetProbeSnapshot(1); snapshot[0].Reachable == nil || *snapshot[0].Reachable {
		t.Fatalf("latest failed probe should remain observable: %#v", snapshot[0])
	}
}

func TestClearProbePreferences_RestoresOriginalOrder(t *testing.T) {
	manager := NewURLManager(0, 3)
	urls := []string{"https://slow.example.com", "https://fast.example.com"}
	manager.ApplyProbeResults(1, urls, []URLProbeResult{
		{URL: urls[0], OK: true, LatencyMs: 240},
		{URL: urls[1], OK: true, LatencyMs: 35},
	})

	manager.ClearProbePreferences()

	ordered := manager.GetSortedURLs(1, urls)
	if ordered[0].URL != urls[0] || ordered[1].URL != urls[1] {
		t.Fatalf("cleared probe preference order = %#v, want original URL order", ordered)
	}
	snapshot := manager.GetProbeSnapshot(1)
	if snapshot[0].Reachable == nil || snapshot[0].LatencyMs == nil {
		t.Fatalf("clearing preferences should preserve probe observations: %#v", snapshot)
	}
}
