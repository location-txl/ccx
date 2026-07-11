<template>
  <section class="cache-stats-page">
    <header class="cache-stats-header">
      <div>
        <div class="text-overline text-medium-emphasis">CODEX / RESPONSES</div>
        <h1 class="text-h5 font-weight-bold">{{ t('cacheStats.title') }}</h1>
        <p class="text-body-2 text-medium-emphasis mb-0">{{ t('cacheStats.description') }}</p>
      </div>
      <div class="d-flex align-center ga-2 flex-wrap">
        <v-btn-toggle v-model="duration" mandatory density="comfortable" variant="outlined" divided :disabled="isLoading">
          <v-btn value="24h">{{ t('cacheStats.lastDay') }}</v-btn>
          <v-btn value="7d">{{ t('cacheStats.lastWeek') }}</v-btn>
          <v-btn value="30d">{{ t('cacheStats.lastMonth') }}</v-btn>
        </v-btn-toggle>
        <v-btn icon="mdi-refresh" variant="text" :loading="isLoading" :disabled="isLoading" :title="t('cacheStats.refresh')" @click="refresh" />
      </div>
    </header>

    <v-alert v-if="errorMessage" type="error" variant="tonal" class="mb-4" closable @click:close="errorMessage = ''">
      {{ errorMessage }}
    </v-alert>

    <div v-if="isLoading" class="loading-state">
      <v-progress-circular indeterminate color="primary" size="32" />
    </div>

    <v-sheet v-else-if="channels.length" border rounded="lg" class="stats-table-wrap">
      <v-table density="comfortable" class="stats-table">
        <thead>
          <tr>
            <th>{{ t('cacheStats.channelModel') }}</th>
            <th class="text-right">{{ t('cacheStats.hitRate') }}</th>
            <th class="text-right">{{ t('cacheStats.cacheRead') }}</th>
            <th class="text-right">{{ t('cacheStats.cacheWrite') }}</th>
            <th class="text-right">{{ t('cacheStats.inputTokens') }}</th>
            <th class="text-right">{{ t('cacheStats.requests') }}</th>
          </tr>
        </thead>
        <tbody>
          <template v-for="channel in channels" :key="channel.channelIndex">
            <tr class="channel-row" :class="{ 'channel-row--open': expandedChannels.has(channel.channelIndex) }">
              <td>
                <div class="d-flex align-center ga-2">
                  <v-btn :icon="expandedChannels.has(channel.channelIndex) ? 'mdi-chevron-down' : 'mdi-chevron-right'" size="x-small" variant="text" :title="expandedChannels.has(channel.channelIndex) ? t('cacheStats.collapse') : t('cacheStats.expand')" @click="toggleChannel(channel.channelIndex)" />
                  <div>
                    <div class="font-weight-medium">{{ channel.channelName || `${t('cacheStats.channel')} ${channel.channelIndex + 1}` }}</div>
                    <div v-if="channel.sharedCredentials" class="shared-note">{{ t('cacheStats.sharedCredentials') }}</div>
                  </div>
                </div>
              </td>
              <td class="text-right"><span class="hit-rate" :class="hitRateClass(channel.summary.cacheHitRate)">{{ formatRate(channel.summary.cacheHitRate) }}</span></td>
              <td class="text-right">{{ formatTokens(channel.summary.cacheReadTokens) }}</td>
              <td class="text-right">{{ formatTokens(channel.summary.cacheCreationTokens) }}</td>
              <td class="text-right">{{ formatTokens(channel.summary.inputTokens) }}</td>
              <td class="text-right">{{ formatNumber(channel.summary.requestCount) }}</td>
            </tr>
            <tr v-for="model in channel.models" v-show="expandedChannels.has(channel.channelIndex)" :key="`${channel.channelIndex}-${model.model}`" class="model-row">
              <td><span class="model-name">{{ model.model }}</span></td>
              <td class="text-right"><span class="hit-rate" :class="hitRateClass(model.cacheHitRate)">{{ formatRate(model.cacheHitRate) }}</span></td>
              <td class="text-right">{{ formatTokens(model.cacheReadTokens) }}</td>
              <td class="text-right">{{ formatTokens(model.cacheCreationTokens) }}</td>
              <td class="text-right">{{ formatTokens(model.inputTokens) }}</td>
              <td class="text-right">{{ formatNumber(model.requestCount) }}</td>
            </tr>
            <tr v-show="expandedChannels.has(channel.channelIndex) && !channel.models.length" class="model-row">
              <td colspan="6" class="text-medium-emphasis text-caption">{{ t('cacheStats.noModels') }}</td>
            </tr>
          </template>
        </tbody>
      </v-table>
    </v-sheet>

    <v-sheet v-else-if="!errorMessage" border rounded="lg" class="empty-state">
      <v-icon size="40" color="grey">mdi-database-search-outline</v-icon>
      <div class="text-subtitle-2 mt-3">{{ t('cacheStats.emptyTitle') }}</div>
      <div class="text-body-2 text-medium-emphasis mt-1">{{ t('cacheStats.emptyDescription') }}</div>
    </v-sheet>
  </section>
</template>

<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import { api, type CacheStatsChannel } from '@/services/api'
import { useI18n } from '@/i18n'

type Duration = '24h' | '7d' | '30d'

const { t } = useI18n()
const duration = ref<Duration>('24h')
const channels = ref<CacheStatsChannel[]>([])
const expandedChannels = ref(new Set<number>())
const isLoading = ref(false)
const errorMessage = ref('')
let requestVersion = 0

const formatNumber = (value: number) => new Intl.NumberFormat().format(value)
const formatTokens = (value: number) => {
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1)}M`
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)}K`
  return formatNumber(value)
}
const formatRate = (rate: number | null) => rate === null ? '--' : `${rate.toFixed(1)}%`
const hitRateClass = (rate: number | null) => {
  if (rate === null) return 'hit-rate--empty'
  if (rate >= 50) return 'hit-rate--high'
  if (rate >= 20) return 'hit-rate--medium'
  return 'hit-rate--low'
}
const toggleChannel = (index: number) => {
  const next = new Set(expandedChannels.value)
  if (next.has(index)) next.delete(index)
  else next.add(index)
  expandedChannels.value = next
}
const refresh = async () => {
  const version = ++requestVersion
  isLoading.value = true
  errorMessage.value = ''
  try {
    const response = await api.getResponsesCacheStats(duration.value)
    if (version !== requestVersion) return
    channels.value = response.channels
    expandedChannels.value = new Set(response.channels.map(channel => channel.channelIndex))
  } catch (error) {
    if (version !== requestVersion) return
    channels.value = []
    errorMessage.value = error instanceof Error ? error.message : t('cacheStats.loadFailed')
  } finally {
    if (version === requestVersion) isLoading.value = false
  }
}

watch(duration, refresh)
onMounted(refresh)
</script>

<style scoped>
.cache-stats-page { max-width: 1280px; margin: 0 auto; }
.cache-stats-header { display: flex; justify-content: space-between; align-items: flex-start; gap: 20px; margin-bottom: 24px; }
.loading-state, .empty-state { min-height: 260px; display: flex; flex-direction: column; align-items: center; justify-content: center; text-align: center; }
.stats-table-wrap { overflow-x: auto; }
.stats-table { min-width: 760px; }
.channel-row { background: rgb(var(--v-theme-surface-variant), 0.25); }
.channel-row--open { border-bottom: 1px solid rgb(var(--v-theme-outline-variant)); }
.model-row td:first-child { padding-left: 56px; }
.model-name { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 0.82rem; }
.shared-note { color: rgb(var(--v-theme-warning)); font-size: 0.72rem; margin-top: 2px; }
.hit-rate { font-variant-numeric: tabular-nums; font-weight: 700; }
.hit-rate--high { color: rgb(var(--v-theme-success)); }
.hit-rate--medium { color: rgb(var(--v-theme-info)); }
.hit-rate--low { color: rgb(var(--v-theme-warning)); }
.hit-rate--empty { color: rgb(var(--v-theme-on-surface-variant)); font-weight: 400; }
@media (max-width: 700px) { .cache-stats-header { flex-direction: column; } }
</style>
