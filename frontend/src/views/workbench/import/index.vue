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

/** Multipart format field sent to API */
type ImportKind = 'psql' | 'pgdump'

const route = useRoute()

const connections = ref<Connection[]>([])
const databases = ref<string[]>([])
const selectedConnId = ref<number | null>(null)
const selectedPhysicalDatabase = ref('')
const importKind = ref<ImportKind>('psql')
const file = ref<File | null>(null)
const busy = ref(false)
const msg = ref('')
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
  importKind.value === 'psql' ? 'Plain SQL (psql)' : 'pg_dump archive',
)

const infoBody = computed(() => {
  if (importKind.value === 'psql') {
    return [
      'Runs the uploaded script with psql -f against the chosen database.',
      'Use for .sql exports created as “Plain SQL” from the Export page, or any pg_dump text output.',
    ]
  }
  return [
    'Restores a custom-format file created by pg_dump -Fc using pg_restore.',
    'Use the archive file (.dump) from the Export page’s “pg_dump archive” option.',
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

function onPickFile(e: Event) {
  const t = e.target as HTMLInputElement
  file.value = t.files?.[0] ?? null
  msg.value = ''
}

async function submitImport() {
  msg.value = ''
  loadErr.value = ''
  if (!selectedConnId.value) {
    msg.value = 'Select a connection'
    return
  }
  if (!file.value) {
    msg.value = 'Choose a file'
    return
  }

  busy.value = true
  try {
    if (isPostgres.value) {
      const formData = new FormData()
      formData.append('file', file.value)
      formData.append('format', importKind.value)
      await http.post(`/api/connections/${selectedConnId.value}/import`, formData, {
        params: dbParams(),
        headers: { 'Content-Type': 'multipart/form-data' },
        timeout: 3_600_000,
      })
      msg.value = 'Import finished successfully.'
    } else {
      const formData = new FormData()
      formData.append('file', file.value)
      await http.post(`/api/connections/${selectedConnId.value}/import`, formData, {
        params: dbParams(),
        headers: { 'Content-Type': 'multipart/form-data' },
        timeout: 3_600_000,
      })
      msg.value = 'SQL import finished successfully.'
    }
  } catch (e: unknown) {
    const err = e as { response?: { data?: { error?: string } } }
    msg.value = err.response?.data?.error || (e as Error).message || 'Import failed'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="io-page">
    <header class="io-header">
      <RouterLink to="/workbench" class="io-back">← Workbench</RouterLink>
      <h1 class="io-title">Import database</h1>
      <p class="io-sub muted">
        PostgreSQL uses <strong>psql</strong> or <strong>pg_restore</strong> on the ChatDB host. MySQL uses the in-app
        SQL executor.
      </p>
    </header>

    <main class="io-main">
      <p v-if="loadErr" class="error">{{ loadErr }}</p>

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

      <template v-if="isPostgres">
        <div class="format-grid">
          <button
            type="button"
            class="format-card"
            :class="{ active: importKind === 'psql' }"
            @click="importKind = 'psql'"
          >
            <span class="format-name">Plain SQL (psql)</span>
            <span class="format-hint muted small">.sql scripts</span>
          </button>
          <button
            type="button"
            class="format-card"
            :class="{ active: importKind === 'pgdump' }"
            @click="importKind = 'pgdump'"
          >
            <span class="format-name">pg_dump archive</span>
            <span class="format-hint muted small">.dump (custom format)</span>
          </button>
        </div>

        <section class="info-panel" aria-live="polite">
          <h2 class="info-title">{{ infoTitle }}</h2>
          <ul class="info-list">
            <li v-for="(line, i) in infoBody" :key="i">{{ line }}</li>
          </ul>
        </section>
      </template>
      <p v-else-if="currentConnection" class="muted small import-mysql-note">
        MySQL: upload runs as a single batch through the app (not mysql client). Large files may be slow.
      </p>

      <div class="upload-block">
        <label class="file-label">
          <span class="file-label-text">Dump file</span>
          <input
            type="file"
            class="file-input"
            :accept="isPostgres && importKind === 'pgdump' ? '.dump,application/octet-stream' : '.sql,.dump'"
            @change="onPickFile"
          />
        </label>
        <p v-if="file" class="file-name muted small">{{ file.name }}</p>

        <button type="button" class="primary import-btn" :disabled="busy || !selectedConnId" @click="submitImport">
          {{ busy ? 'Importing…' : 'Import' }}
        </button>
        <p v-if="msg" class="msg" :class="{ error: msg.includes('fail') || msg.includes('Error') }">{{ msg }}</p>
      </div>
    </main>
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
.io-main {
  padding: 1rem 1.25rem 2rem;
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
  margin-bottom: 1.25rem;
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
.import-mysql-note {
  margin-bottom: 1rem;
}
.upload-block {
  padding: 1rem;
  border-radius: 8px;
  border: 1px solid #30363d;
  background: #161b22;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}
.file-label {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  font-size: 0.8rem;
  color: #8b949e;
}
.file-input {
  font-size: 0.8rem;
  color: #e6edf3;
}
.file-name {
  margin: 0;
}
.import-btn {
  align-self: flex-start;
  padding: 0.5rem 1.2rem;
  border-radius: 8px;
  border: none;
  font-weight: 600;
  cursor: pointer;
  background: #238636;
  color: #fff;
}
.import-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.msg {
  margin: 0;
  font-size: 0.85rem;
  color: #3fb950;
}
.msg.error {
  color: #f85149;
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
</style>
