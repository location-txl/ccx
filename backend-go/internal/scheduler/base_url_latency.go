package scheduler

import (
	"context"
	"net"
	"net/url"
	"sort"
	"sync"
	"time"

	"github.com/BenedictKing/ccx/internal/config"
	"github.com/BenedictKing/ccx/internal/warmup"
)

const (
	baseURLProbeInterval    = 5 * time.Minute
	baseURLProbeTimeout     = 5 * time.Second
	baseURLProbeConcurrency = 10
)

// BaseURLLatencyChannel 是管理端展示一个渠道的多 URL 测速快照。
type BaseURLLatencyChannel struct {
	Kind         ChannelKind               `json:"kind"`
	ChannelIndex int                       `json:"channelIndex"`
	ChannelName  string                    `json:"channelName"`
	URLs         []warmup.URLProbeSnapshot `json:"urls"`
}

// BaseURLLatencySnapshot 是管理端的全局多 URL 测速快照。
type BaseURLLatencySnapshot struct {
	Enabled         bool                    `json:"enabled"`
	IntervalSeconds int64                   `json:"intervalSeconds"`
	LastRunAt       time.Time               `json:"lastRunAt,omitempty"`
	Channels        []BaseURLLatencyChannel `json:"channels"`
}

type baseURLProbeTarget struct {
	kind         ChannelKind
	channelIndex int
	upstream     config.UpstreamConfig
	urls         []string
}

// BaseURLLatencyProber 定期探测多 Base URL 的 TCP 连接延迟。
type BaseURLLatencyProber struct {
	cfgManager *config.ConfigManager
	scheduler  *ChannelScheduler

	mu        sync.Mutex
	running   bool
	started   bool
	triggerCh chan struct{}
	stopCh    chan struct{}
	lastRunAt time.Time
}

func NewBaseURLLatencyProber(cfgManager *config.ConfigManager, channelScheduler *ChannelScheduler) *BaseURLLatencyProber {
	return &BaseURLLatencyProber{
		cfgManager: cfgManager,
		scheduler:  channelScheduler,
		triggerCh:  make(chan struct{}, 1),
		stopCh:     make(chan struct{}),
	}
}

// Start 启动后台任务。真正的探测由配置开关控制。
func (p *BaseURLLatencyProber) Start() {
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return
	}
	p.started = true
	p.mu.Unlock()

	go func() {
		ticker := time.NewTicker(baseURLProbeInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				p.runIfEnabled()
			case <-p.triggerCh:
				p.runIfEnabled()
			case <-p.stopCh:
				return
			}
		}
	}()
	p.Trigger()
}

// Stop 仅供优雅退出和测试使用。
func (p *BaseURLLatencyProber) Stop() {
	p.mu.Lock()
	if !p.started {
		p.mu.Unlock()
		return
	}
	p.started = false
	close(p.stopCh)
	p.mu.Unlock()
}

// Trigger 请求立即执行一轮测速；重复请求会合并。
func (p *BaseURLLatencyProber) Trigger() {
	select {
	case p.triggerCh <- struct{}{}:
	default:
	}
}

func (p *BaseURLLatencyProber) runIfEnabled() {
	if p.cfgManager == nil || !p.cfgManager.GetBaseURLLatencyProbeEnabled() {
		return
	}
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return
	}
	p.running = true
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		p.running = false
		p.lastRunAt = time.Now()
		p.mu.Unlock()
	}()

	cfg := p.cfgManager.GetConfig()
	targets := collectBaseURLProbeTargets(cfg)
	if len(targets) == 0 {
		return
	}

	sem := make(chan struct{}, baseURLProbeConcurrency)
	var wg sync.WaitGroup
	for _, target := range targets {
		results := make([]warmup.URLProbeResult, len(target.urls))
		for index, baseURL := range target.urls {
			wg.Add(1)
			go func(i int, rawURL string, target baseURLProbeTarget) {
				defer wg.Done()
				sem <- struct{}{}
				results[i] = probeBaseURLTCP(rawURL)
				<-sem
			}(index, baseURL, target)
		}
		wg.Wait()
		if !p.cfgManager.GetBaseURLLatencyProbeEnabled() {
			return
		}
		p.scheduler.ApplyBaseURLProbeResults(target.kind, target.channelIndex, target.urls, results)
	}
}

func collectBaseURLProbeTargets(cfg config.Config) []baseURLProbeTarget {
	groups := []struct {
		kind      ChannelKind
		upstreams []config.UpstreamConfig
	}{
		{ChannelKindMessages, cfg.Upstream},
		{ChannelKindChat, cfg.ChatUpstream},
		{ChannelKindResponses, cfg.ResponsesUpstream},
		{ChannelKindGemini, cfg.GeminiUpstream},
		{ChannelKindImages, cfg.ImagesUpstream},
		{ChannelKindVectors, cfg.VectorsUpstream},
	}

	var targets []baseURLProbeTarget
	for _, group := range groups {
		for index, upstream := range group.upstreams {
			if config.GetChannelStatus(&upstream) != "active" {
				continue
			}
			urls := upstream.GetAllBaseURLs()
			if len(urls) > 1 {
				targets = append(targets, baseURLProbeTarget{kind: group.kind, channelIndex: index, upstream: upstream, urls: urls})
			}
		}
	}
	return targets
}

func probeBaseURLTCP(rawURL string) warmup.URLProbeResult {
	result := warmup.URLProbeResult{URL: rawURL}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" {
		result.Error = "invalid_url"
		return result
	}

	port := parsed.Port()
	if port == "" {
		if parsed.Scheme == "https" {
			port = "443"
		} else if parsed.Scheme == "http" {
			port = "80"
		} else {
			result.Error = "invalid_url"
			return result
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), baseURLProbeTimeout)
	defer cancel()
	start := time.Now()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(parsed.Hostname(), port))
	elapsed := time.Since(start).Milliseconds()
	if err != nil {
		if ctx.Err() != nil {
			result.Error = "timeout"
		} else {
			result.Error = "network_error"
		}
		return result
	}
	_ = conn.Close()
	result.OK = true
	result.LatencyMs = elapsed
	return result
}

// ApplyBaseURLProbeResults 更新指定渠道的 URL 探测结果。
func (s *ChannelScheduler) ApplyBaseURLProbeResults(kind ChannelKind, channelIndex int, urls []string, results []warmup.URLProbeResult) {
	if s == nil || s.urlManager == nil {
		return
	}
	s.urlManager.ApplyProbeResults(urlManagerChannelKey(kind, channelIndex), urls, results)
}

// ClearBaseURLProbePreferences 停止使用已缓存的后台测速结果排序。
func (s *ChannelScheduler) ClearBaseURLProbePreferences() {
	if s == nil || s.urlManager == nil {
		return
	}
	s.urlManager.ClearProbePreferences()
}

// GetBaseURLLatencySnapshot 返回所有多 Base URL 渠道的最近探测快照。
func (s *ChannelScheduler) GetBaseURLLatencySnapshot() BaseURLLatencySnapshot {
	snapshot := BaseURLLatencySnapshot{Enabled: s != nil && s.configManager != nil && s.configManager.GetBaseURLLatencyProbeEnabled(), IntervalSeconds: int64(baseURLProbeInterval.Seconds())}
	if s == nil || s.configManager == nil || s.urlManager == nil {
		return snapshot
	}

	groups := []struct {
		kind      ChannelKind
		upstreams []config.UpstreamConfig
	}{
		{ChannelKindMessages, s.configManager.GetConfig().Upstream},
		{ChannelKindChat, s.configManager.GetConfig().ChatUpstream},
		{ChannelKindResponses, s.configManager.GetConfig().ResponsesUpstream},
		{ChannelKindGemini, s.configManager.GetConfig().GeminiUpstream},
		{ChannelKindImages, s.configManager.GetConfig().ImagesUpstream},
		{ChannelKindVectors, s.configManager.GetConfig().VectorsUpstream},
	}
	for _, group := range groups {
		for index, upstream := range group.upstreams {
			if len(upstream.GetAllBaseURLs()) <= 1 {
				continue
			}
			urls := s.urlManager.GetProbeSnapshot(urlManagerChannelKey(group.kind, index))
			snapshot.Channels = append(snapshot.Channels, BaseURLLatencyChannel{Kind: group.kind, ChannelIndex: index, ChannelName: upstream.Name, URLs: urls})
			for _, entry := range urls {
				if entry.ProbedAt.After(snapshot.LastRunAt) {
					snapshot.LastRunAt = entry.ProbedAt
				}
			}
		}
	}
	sort.Slice(snapshot.Channels, func(i, j int) bool {
		if snapshot.Channels[i].Kind != snapshot.Channels[j].Kind {
			return snapshot.Channels[i].Kind < snapshot.Channels[j].Kind
		}
		return snapshot.Channels[i].ChannelIndex < snapshot.Channels[j].ChannelIndex
	})
	return snapshot
}
