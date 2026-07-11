<template>
  <section class="base-url-latency-page">
    <header class="latency-header">
      <div>
        <div class="text-overline text-medium-emphasis">NETWORK / BASE URL</div>
        <h1 class="text-h5 font-weight-bold">{{ t('baseUrlLatency.title') }}</h1>
        <p class="text-body-2 text-medium-emphasis mb-0">{{ t('baseUrlLatency.description') }}</p>
      </div>
      <div class="d-flex align-center ga-2 flex-wrap">
        <v-chip :color="data.enabled ? 'success' : 'warning'" variant="tonal" size="small">
          <v-icon start size="16">{{ data.enabled ? 'mdi-radar' : 'mdi-radar-off' }}</v-icon>
          {{ data.enabled ? t('baseUrlLatency.enabled') : t('baseUrlLatency.disabled') }}
        </v-chip>
        <span v-if="data.enabled" class="text-caption text-medium-emphasis">{{ t('baseUrlLatency.interval', { seconds: data.intervalSeconds }) }}</span>
        <v-btn icon="mdi-refresh" variant="text" :loading="loading" :title="t('baseUrlLatency.refresh')" @click="refresh" />
      </div>
    </header>

    <v-alert v-if="errorMessage" type="error" variant="tonal" class="mb-4" closable @click:close="errorMessage = ''">{{ errorMessage }}</v-alert>
    <v-alert v-if="!data.enabled" type="info" variant="tonal" class="mb-4">{{ t('baseUrlLatency.disabledHint') }}</v-alert>

    <div class="kind-filter" role="tablist">
      <v-btn v-for="option in filterOptions" :key="option.value" :variant="activeKind === option.value ? 'flat' : 'text'" :color="activeKind === option.value ? 'primary' : undefined" size="small" @click="activeKind = option.value">{{ option.label }}</v-btn>
    </div>

    <div v-if="loading && !hasEntries" class="loading-state"><v-progress-circular indeterminate color="primary" size="32" /></div>
    <v-sheet v-else-if="visibleChannels.length" border rounded="lg" class="latency-table-wrap">
      <v-table density="comfortable" class="latency-table">
        <thead><tr><th>{{ t('baseUrlLatency.channel') }}</th><th>{{ t('baseUrlLatency.baseUrl') }}</th><th>{{ t('baseUrlLatency.status') }}</th><th class="text-right">{{ t('baseUrlLatency.latency') }}</th><th class="text-right">{{ t('baseUrlLatency.priority') }}</th><th>{{ t('baseUrlLatency.updatedAt') }}</th></tr></thead>
        <tbody>
          <template v-for="channel in visibleChannels" :key="`${channel.kind}-${channel.channelIndex}`">
            <tr v-for="entry in channel.urls" :key="entry.url" :class="{ 'preferred-row': entry.preferred }">
              <td><div class="font-weight-medium">{{ channel.channelName || `${channel.kind} #${channel.channelIndex + 1}` }}</div><div class="text-caption text-medium-emphasis text-uppercase">{{ channel.kind }}</div></td>
              <td><code class="url-cell" :title="entry.url">{{ entry.url }}</code></td>
              <td><v-chip :color="statusColor(entry)" size="x-small" variant="tonal"><v-icon start size="14">{{ statusIcon(entry) }}</v-icon>{{ statusText(entry) }}</v-chip></td>
              <td class="text-right"><span :class="latencyClass(entry.latencyMs)">{{ entry.latencyMs === undefined ? '--' : `${entry.latencyMs} ms` }}</span></td>
              <td class="text-right"><v-chip v-if="entry.preferred" color="primary" size="x-small" variant="flat">{{ t('baseUrlLatency.preferred') }}</v-chip><span v-else>#{{ entry.rank }}</span></td>
              <td class="text-caption text-medium-emphasis">{{ formatTime(entry.probedAt) }}</td>
            </tr>
          </template>
        </tbody>
      </v-table>
    </v-sheet>
    <v-sheet v-else-if="!errorMessage" border rounded="lg" class="empty-state"><v-icon size="40" color="grey">mdi-radar</v-icon><div class="text-subtitle-2 mt-3">{{ t('baseUrlLatency.empty') }}</div></v-sheet>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { api, type BaseURLLatencyEntry, type BaseURLLatencyResponse, type ChannelKind } from '@/services/api'
import { useI18n } from '@/i18n'

const { t } = useI18n()
const kinds: Array<'all' | ChannelKind> = ['all', 'messages', 'chat', 'responses', 'gemini', 'images', 'vectors']
const activeKind = ref<'all' | ChannelKind>('all')
const data = ref<BaseURLLatencyResponse>({ enabled: false, intervalSeconds: 300, channels: [] })
const loading = ref(false)
const errorMessage = ref('')
let refreshTimer: ReturnType<typeof window.setInterval> | undefined

const filterOptions = computed(() => kinds.map(kind => ({ value: kind, label: kind === 'all' ? t('baseUrlLatency.all') : kind })))
const visibleChannels = computed(() => data.value.channels.filter(channel => activeKind.value === 'all' || channel.kind === activeKind.value))
const hasEntries = computed(() => data.value.channels.some(channel => channel.urls.length > 0))

const refresh = async () => {
  loading.value = true
  try {
    data.value = await api.getBaseURLLatency()
    errorMessage.value = ''
  } catch (error) {
    errorMessage.value = t('baseUrlLatency.loadFailed', { message: error instanceof Error ? error.message : String(error) })
  } finally {
    loading.value = false
  }
}

const statusText = (entry: BaseURLLatencyEntry) => entry.reachable === undefined ? t('baseUrlLatency.unprobed') : entry.reachable ? t('baseUrlLatency.reachable') : t('baseUrlLatency.unreachable')
const statusColor = (entry: BaseURLLatencyEntry) => entry.reachable === undefined ? 'grey' : entry.reachable ? 'success' : 'error'
const statusIcon = (entry: BaseURLLatencyEntry) => entry.reachable === undefined ? 'mdi-clock-outline' : entry.reachable ? 'mdi-check-circle-outline' : 'mdi-alert-circle-outline'
const latencyClass = (latency?: number) => latency === undefined ? 'text-medium-emphasis' : latency < 100 ? 'latency-fast' : latency < 300 ? 'latency-medium' : 'latency-slow'
const formatTime = (value?: string) => value ? new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(new Date(value)) : '--'

onMounted(() => { void refresh(); refreshTimer = window.setInterval(() => void refresh(), 5_000) })
onBeforeUnmount(() => { if (refreshTimer) window.clearInterval(refreshTimer) })
</script>

<style scoped>
.base-url-latency-page { display: grid; gap: 16px; }
.latency-header { display: flex; justify-content: space-between; gap: 16px; align-items: flex-start; }
.kind-filter { display: flex; flex-wrap: wrap; gap: 4px; border-bottom: 1px solid rgb(var(--v-theme-outline-variant)); padding-bottom: 8px; }
.latency-table-wrap { overflow-x: auto; }
.latency-table { min-width: 780px; }
.preferred-row { background: rgba(var(--v-theme-primary), 0.07); }
.url-cell { display: block; max-width: 460px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 12px; }
.latency-fast { color: rgb(var(--v-theme-success)); font-weight: 700; }
.latency-medium { color: rgb(var(--v-theme-warning)); font-weight: 700; }
.latency-slow { color: rgb(var(--v-theme-error)); font-weight: 700; }
.loading-state, .empty-state { min-height: 220px; display: grid; place-content: center; text-align: center; }
@media (max-width: 700px) { .latency-header { flex-direction: column; } }
</style>
