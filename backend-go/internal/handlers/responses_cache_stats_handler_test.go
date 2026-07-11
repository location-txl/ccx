package handlers

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/BenedictKing/ccx/internal/config"
	"github.com/BenedictKing/ccx/internal/metrics"
	"github.com/gin-gonic/gin"
)

func TestGetResponsesCacheStats_GroupsModelsAndMarksSharedCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	baseURL := "https://codex.example.com"
	apiKey := "sk-shared"
	metricsKey := metrics.GenerateMetricsIdentityKey(baseURL, apiKey, "responses")
	store := &fakePersistenceStore{modelBucketsByMetricsKey: map[string][]metrics.ModelAggregatedBucket{
		metricsKey: {
			{Model: "gpt-5", TotalRequests: 3, InputTokens: 100, CacheReadTokens: 300, CacheCreationTokens: 50},
			{Model: "gpt-5", TotalRequests: 2, InputTokens: 100, CacheReadTokens: 100, CacheCreationTokens: 20},
			{Model: "gpt-5-mini", TotalRequests: 1, InputTokens: 400, CacheReadTokens: 0},
		},
	}}
	metricsManager := metrics.NewMetricsManagerWithPersistence(20, 0.7, store, "responses")
	defer metricsManager.Stop()

	configFile := filepath.Join(t.TempDir(), "config.json")
	cfg := config.Config{ResponsesUpstream: []config.UpstreamConfig{
		{Name: "Codex A", BaseURL: baseURL, APIKeys: []string{apiKey}, ServiceType: "responses"},
		{Name: "Codex B", BaseURL: baseURL, APIKeys: []string{apiKey}, ServiceType: "responses"},
	}}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configFile, data, 0600); err != nil {
		t.Fatal(err)
	}
	cfgManager, err := config.NewConfigManager(configFile, "")
	if err != nil {
		t.Fatal(err)
	}
	defer cfgManager.Close()

	router := gin.New()
	router.GET("/responses/cache-stats", GetResponsesCacheStats(metricsManager, cfgManager))
	req := httptest.NewRequest(http.MethodGet, "/responses/cache-stats?duration=7d", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", resp.Code, resp.Body.String())
	}

	var body struct {
		Duration string              `json:"duration"`
		Channels []cacheStatsChannel `json:"channels"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Duration != "7d" || len(body.Channels) != 2 {
		t.Fatalf("unexpected response: %s", resp.Body.String())
	}
	for _, channel := range body.Channels {
		if !channel.SharedCredentials {
			t.Fatalf("channel %q should be marked shared", channel.ChannelName)
		}
		if channel.Summary.CacheHitRate == nil || *channel.Summary.CacheHitRate != 40 {
			t.Fatalf("channel %q hit rate = %v, want 40", channel.ChannelName, channel.Summary.CacheHitRate)
		}
		if len(channel.Models) != 2 || channel.Models[0].Model != "gpt-5" || channel.Models[0].CacheHitRate == nil || math.Abs(*channel.Models[0].CacheHitRate-66.6666666667) > 0.0001 {
			t.Fatalf("channel %q models = %+v", channel.ChannelName, channel.Models)
		}
	}
}

func TestGetResponsesCacheStats_RejectsUnsupportedDurationAndMissingStore(t *testing.T) {
	gin.SetMode(gin.TestMode)
	configFile := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configFile, []byte(`{"responsesUpstream":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfgManager, err := config.NewConfigManager(configFile, "")
	if err != nil {
		t.Fatal(err)
	}
	defer cfgManager.Close()
	metricsManager := metrics.NewMetricsManager()
	defer metricsManager.Stop()

	router := gin.New()
	router.GET("/responses/cache-stats", GetResponsesCacheStats(metricsManager, cfgManager))
	for _, path := range []string{"/responses/cache-stats?duration=1h", "/responses/cache-stats?duration=30d"} {
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, path, nil))
		if resp.Code != http.StatusBadRequest {
			t.Fatalf("%s status=%d, body=%s", path, resp.Code, resp.Body.String())
		}
	}
}
