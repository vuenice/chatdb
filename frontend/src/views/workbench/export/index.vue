<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { useRoute, RouterLink } from 'vue-router'
import { http } from '../../../api/http'

interface Connection {
  id: number
  name: string
  driver: string
  database: string
}

/** UI maps to API: plain = text SQL for psql; archive = pg_dump -Fc */
type ArchiveKind = 'plain' | 'archive'

const route = useRoute()

const connections = ref<Connection[]>([])
const databases = ref<string[]>([])
const selectedConnId = ref<number | null>(null)
const selectedPhysicalDatabase = ref('')
const archiveKind = ref<ArchiveKind>('plain')
const busy = ref(false)
const loadErr = ref('')

function dbParams(): Record<string, string> {
  const o: Record<string, string> = {}
  if (selectedPhysicalDatabase.value) o.database = selectedPhysicalDatabase.value
  return o
}

const currentConnection = computed(
  () => connections.value.find((c) => c.id === selectedConnId.value) ?? null,
)

const isPostgres = computed(() => currentConnection.value?.driver === 'postgres')

const infoTitle = computed(() =>
  archiveKind.value === 'plain' ? 'Plain SQL (psql)' : 'pg_dump archive',
)

const infoBody = computed(() => {
  if (archiveKind.value === 'plain') {
    return [
      'Produces a readable .sql script using pg_dump in plain text mode.',
      'Restore anywhere with psql -f dump.sql or use the Import page with type “Plain SQL (psql)”.',
      'Equivalent: pg_dump -h … -p … -U … --no-owner -d … > db.sql',
    ]
  }
  return [
    'Produces a PostgreSQL custom-format binary file (pg_dump -Fc).',
    'Smaller and faster restores for large databases; restored with pg_restore, not plain psql.',
    'Use the Import page with type “pg_dump archive” for this file.',
  ]
})

function applyRouteQuery() {
  const cid = route.query.connection
  if (cid != null && cid !== '') {
    const id = Number(cid)
    if (!Number.isNaN(id)) selectedConnId.value = id
  }
  const db = route.query.database
  if (typeof db === 'string' && db) {
    selectedPhysicalDatabase.value = db
  }
}

async function loadConnections() {
  loadErr.value = ''
  try {
    const { data } = await http.get<{ connections: Connection[] }>('/api/connections')
    connections.value = data.connections || []
    applyRouteQuery()
    if (selectedConnId.value == null && connections.value.length) {
      selectedConnId.value = connections.value[0].id
      if (!selectedPhysicalDatabase.value) {
        selectedPhysicalDatabase.value = connections.value[0].database || ''
      }
    }
  } catch (e: unknown) {
    loadErr.value = (e as { message?: string })?.message || 'Failed to load connections'
  }
}

async function loadDatabases() {
  if (!selectedConnId.value) {
    databases.value = []
    return
  }
  try {
    const { data } = await http.get<{ databases: string[] }>(
      `/api/connections/${selectedConnId.value}/databases`,
      { params: dbParams() },
    )
    databases.value = data.databases || []
  } catch {
    databases.value = []
  }
}

watch(selectedConnId, async () => {
  if (
    selectedConnId.value &&
    currentConnection.value &&
    !selectedPhysicalDatabase.value.trim()
  ) {
    selectedPhysicalDatabase.value = currentConnection.value.database || ''
  }
  await loadDatabases()
})

watch(
  () => [selectedConnId.value, selectedPhysicalDatabase.value],
  async () => {
    await loadDatabases()
  },
)

onMounted(async () => {
  await loadConnections()
  await loadDatabases()
})

async function parseExportError(blob: Blob): Promise<string> {
  try {
    const t = await blob.text()
    const j = JSON.parse(t) as { error?: string }
    return j.error || t.slice(0, 500)
  } catch {
    return 'Export failed'
  }
}

function safeDownloadBase(): string {
  const db =
    selectedPhysicalDatabase.value.trim() ||
    currentConnection.value?.database ||
    'database'
  return db.replace(/[^\w\-]+/g, '_').slice(0, 120) || 'database'
}

async function download() {
  loadErr.value = ''
  if (!selectedConnId.value) {
    loadErr.value = 'Select a connection'
    return
  }
  busy.value = true
  try {
    const format =
      isPostgres.value && archiveKind.value === 'archive' ? 'archive' : 'plain'
    const response = await http.get(`/api/connections/${selectedConnId.value}/export`, {
      params: { format, ...dbParams() },
      responseType: 'blob',
      timeout: 3_600_000,
      validateStatus: (s) => s < 500,
    })
    if (response.status !== 200) {
      const msg = await parseExportError(response.data as Blob)
      loadErr.value = msg
      return
    }
    const blob = response.data as Blob
    const ext = format === 'archive' ? '.dump' : '.sql'
    const url = window.URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `${safeDownloadBase()}${ext}`
    a.click()
    window.URL.revokeObjectURL(url)
  } catch (e: unknown) {
    loadErr.value = (e as { message?: string })?.message || 'Export failed'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="io-page">
    <header class="io-header">
      <RouterLink to="/workbench" class="io-back">← Workbench</RouterLink>
      <h1 class="io-title">Export database</h1>
      <p class="io-sub muted">
        PostgreSQL dumps use <strong>pg_dump</strong> on the server running ChatDB (<code>postgresql-client</code> on
        <code>PATH</code>).
      </p>
    </header>

    <main class="io-main">
      <p v-if="loadErr && !busy" class="error">{{ loadErr }}</p>

      <div class="io-field">
        <label>Connection</label>
        <select v-model.number="selectedConnId" class="io-input">
          <option disabled :value="null">Choose…</option>
          <option v-for="c in connections" :key="c.id" :value="c.id">
            {{ c.name }} ({{ c.driver }})
          </option>
        </select>
      </div>

      <div class="io-field">
        <label>Database</label>
        <select v-model="selectedPhysicalDatabase" class="io-input">
          <option value="">(connection default)</option>
          <option v-for="d in databases" :key="d" :value="d">{{ d }}</option>
        </select>
      </div>

      <div class="format-grid">
        <button
          type="button"
          class="format-card"
          :class="{ active: archiveKind === 'plain' }"
          @click="archiveKind = 'plain'"
        >
          <span class="format-name">Plain SQL (psql)</span>
          <span class="format-hint muted small">Human-readable .sql</span>
        </button>
        <button
          type="button"
          class="format-card"
          :class="{ active: archiveKind === 'archive' }"
          @click="archiveKind = 'archive'"
        >
          <span class="format-name">pg_dump archive</span>
          <span class="format-hint muted small">Custom format (.dump)</span>
        </button>
      </div>

      <section class="info-panel" aria-live="polite">
        <h2 class="info-title">{{ infoTitle }}</h2>
        <ul class="info-list">
          <li v-for="(line, i) in infoBody" :key="i">{{ line }}</li>
        </ul>
        <p v-if="!isPostgres && currentConnection" class="muted small">
          MySQL connections still download the lightweight table listing from the API until a mysqldump path is added.
        </p>
      </section>

      <!-- reserve space above fixed footer -->
      <div class="io-footer-spacer" aria-hidden="true" />
    </main>

    <footer class="io-footer-fixed">
      <button
        type="button"
        class="primary footer-btn"
        :disabled="busy || !selectedConnId"
        @click="download"
      >
        {{ busy ? 'Preparing download…' : 'Download dump' }}
      </button>
    </footer>
  </div>
</template>

<style scoped>
.io-page {
  min-height: 100vh;
  background: #0d1117;
  color: #e6edf3;
}
.io-header {
  padding: 1rem 1.25rem 0.5rem;
  border-bottom: 1px solid #30363d;
}
.io-back {
  color: #58a6ff;
  text-decoration: none;
  font-size: 0.85rem;
}
.io-title {
  margin: 0.75rem 0 0.25rem;
  font-size: 1.25rem;
}
.io-sub {
  margin: 0 0 0.75rem;
  font-size: 0.8rem;
  line-height: 1.4;
}
.io-sub code {
  font-size: 0.76rem;
}
.io-main {
  padding: 1rem 1.25rem;
  max-width: 560px;
}
.io-field {
  margin-bottom: 1rem;
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  font-size: 0.8rem;
  color: #8b949e;
}
.io-input {
  padding: 0.45rem 0.5rem;
  border-radius: 6px;
  border: 1px solid #30363d;
  background: #161b22;
  color: #e6edf3;
}
.format-grid {
  display: grid;
  gap: 0.65rem;
  margin-bottom: 1.25rem;
}
.format-card {
  text-align: left;
  padding: 0.85rem 1rem;
  border-radius: 8px;
  border: 1px solid #30363d;
  background: #161b22;
  color: inherit;
  cursor: pointer;
  transition:
    border-color 0.15s,
    background 0.15s;
}
.format-card:hover {
  border-color: #58a6ff;
}
.format-card.active {
  border-color: #58a6ff;
  background: #21262d;
}
.format-name {
  display: block;
  font-weight: 600;
  font-size: 0.95rem;
}
.format-hint {
  display: block;
  margin-top: 0.2rem;
}
.info-panel {
  padding: 1rem;
  border-radius: 8px;
  border: 1px solid #30363d;
  background: #161b22;
}
.info-title {
  margin: 0 0 0.5rem;
  font-size: 0.95rem;
}
.info-list {
  margin: 0;
  padding-left: 1.1rem;
  font-size: 0.82rem;
  line-height: 1.55;
  color: #c9d1d9;
}
.error {
  color: #f85149;
  font-size: 0.85rem;
  margin-bottom: 0.75rem;
}
.muted {
  color: #8b949e;
}
.small {
  font-size: 0.78rem;
}
.io-footer-spacer {
  height: 5rem;
}
.io-footer-fixed {
  position: fixed;
  left: 0;
  right: 0;
  bottom: 0;
  padding: 0.75rem 1.25rem calc(0.85rem + env(safe-area-inset-bottom));
  border-top: 1px solid #30363d;
  background: #161b22;
  display: flex;
  justify-content: center;
  z-index: 20;
}
.footer-btn {
  min-width: 220px;
  padding: 0.55rem 1.25rem;
  border-radius: 8px;
  border: none;
  font-weight: 600;
  cursor: pointer;
  background: #238636;
  color: #fff;
}
.footer-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
