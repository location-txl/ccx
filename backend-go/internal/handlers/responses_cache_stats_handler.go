package handlers

import (
	"net/http"
	"sort"
	"time"

	"github.com/BenedictKing/ccx/internal/config"
	"github.com/BenedictKing/ccx/internal/metrics"
	"github.com/BenedictKing/ccx/internal/scheduler"
	"github.com/gin-gonic/gin"
)

type cacheStatsSummary struct {
	RequestCount        int64    `json:"requestCount"`
	InputTokens         int64    `json:"inputTokens"`
	CacheReadTokens     int64    `json:"cacheReadTokens"`
	CacheCreationTokens int64    `json:"cacheCreationTokens"`
	CacheHitRate        *float64 `json:"cacheHitRate"`
}

type cacheStatsModel struct {
	Model string `json:"model"`
	cacheStatsSummary
}

type cacheStatsChannel struct {
	ChannelIndex      int               `json:"channelIndex"`
	ChannelName       string            `json:"channelName"`
	SharedCredentials bool              `json:"sharedCredentials"`
	Summary           cacheStatsSummary `json:"summary"`
	Models            []cacheStatsModel `json:"models"`
}

// GetResponsesCacheStats returns long-range Codex cache statistics grouped by channel and model.
func GetResponsesCacheStats(metricsManager *metrics.MetricsManager, cfgManager *config.ConfigManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		durationLabel := c.DefaultQuery("duration", "24h")
		duration, ok := map[string]time.Duration{
			"24h": 24 * time.Hour,
			"7d":  7 * 24 * time.Hour,
			"30d": 30 * 24 * time.Hour,
		}[durationLabel]
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid duration parameter. Use: 24h, 7d, or 30d"})
			return
		}

		store := metricsManager.GetPersistenceStore()
		if store == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "缓存统计需要启用 SQLite 指标持久化存储"})
			return
		}

		upstreams := cfgManager.GetConfig().ResponsesUpstream
		sharedKeys := sharedResponseMetricsKeys(upstreams)
		since := time.Now().Add(-duration)
		intervalSeconds := int64(duration.Seconds())
		channels := make([]cacheStatsChannel, 0, len(upstreams))

		for index, upstream := range upstreams {
			serviceType := scheduler.NormalizedMetricsServiceType(scheduler.ChannelKindResponses, upstream.ServiceType)
			buckets := filterModelBucketsByURLs(
				store,
				metricsManager.GetAPIType(),
				since,
				intervalSeconds,
				upstream.GetAllBaseURLs(),
				channelStatsAPIKeys(upstream),
				serviceType,
			)

			modelsByName := make(map[string]*cacheStatsModel)
			for _, bucket := range buckets {
				if bucket.Model == "" {
					continue
				}
				model := modelsByName[bucket.Model]
				if model == nil {
					model = &cacheStatsModel{Model: bucket.Model}
					modelsByName[bucket.Model] = model
				}
				model.RequestCount += bucket.TotalRequests
				model.InputTokens += bucket.InputTokens
				model.CacheReadTokens += bucket.CacheReadTokens
				model.CacheCreationTokens += bucket.CacheCreationTokens
			}

			models := make([]cacheStatsModel, 0, len(modelsByName))
			summary := cacheStatsSummary{}
			for _, model := range modelsByName {
				finalizeCacheHitRate(&model.cacheStatsSummary)
				summary.RequestCount += model.RequestCount
				summary.InputTokens += model.InputTokens
				summary.CacheReadTokens += model.CacheReadTokens
				summary.CacheCreationTokens += model.CacheCreationTokens
				models = append(models, *model)
			}
			finalizeCacheHitRate(&summary)
			sort.Slice(models, func(i, j int) bool {
				if models[i].CacheReadTokens == models[j].CacheReadTokens {
					return models[i].Model < models[j].Model
				}
				return models[i].CacheReadTokens > models[j].CacheReadTokens
			})

			channels = append(channels, cacheStatsChannel{
				ChannelIndex:      index,
				ChannelName:       upstream.Name,
				SharedCredentials: channelUsesSharedMetricsKey(upstream, serviceType, sharedKeys),
				Summary:           summary,
				Models:            models,
			})
		}

		sort.SliceStable(channels, func(i, j int) bool {
			left, right := channels[i].Summary.CacheHitRate, channels[j].Summary.CacheHitRate
			if left == nil {
				return false
			}
			if right == nil {
				return true
			}
			return *left > *right
		})

		c.JSON(http.StatusOK, gin.H{"duration": durationLabel, "channels": channels})
	}
}

func finalizeCacheHitRate(summary *cacheStatsSummary) {
	denominator := summary.InputTokens + summary.CacheReadTokens
	if denominator <= 0 {
		return
	}
	rate := float64(summary.CacheReadTokens) / float64(denominator) * 100
	summary.CacheHitRate = &rate
}

func sharedResponseMetricsKeys(upstreams []config.UpstreamConfig) map[string]bool {
	counts := make(map[string]int)
	for _, upstream := range upstreams {
		serviceType := scheduler.NormalizedMetricsServiceType(scheduler.ChannelKindResponses, upstream.ServiceType)
		for _, key := range channelMetricsKeys(&upstream, serviceType) {
			counts[key]++
		}
	}
	shared := make(map[string]bool)
	for key, count := range counts {
		if count > 1 {
			shared[key] = true
		}
	}
	return shared
}

func channelUsesSharedMetricsKey(upstream config.UpstreamConfig, serviceType string, sharedKeys map[string]bool) bool {
	for _, key := range channelMetricsKeys(&upstream, serviceType) {
		if sharedKeys[key] {
			return true
		}
	}
	return false
}
