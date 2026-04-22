<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import { http } from '../api/http'
import { useAuthStore } from '../stores/auth'

const router = useRouter()
const auth = useAuthStore()

type Nav = 'tables' | 'queries' | 'users' | 'chat'
type QueriesTab = 'saved' | 'recent' | 'running'
type TableTab = 'structure' | 'data' | 'indexes'

interface Connection {
  id: number
  name: string
  driver: string
  host: string
  port: number
  database: string
  ssl_mode: string
  read_username: string
  write_username: string
  allowed_schemas: string[]
}

interface TableMeta {
  schema: string
  name: string
  kind: string
}

const nav = ref<Nav>('tables')
const chatPinned = ref(false)
const connections = ref<Connection[]>([])
const selectedConnId = ref<number | null>(null)
const databases = ref<string[]>([])
/** Physical database override; empty = omit query param (server uses stored default). */
const selectedPhysicalDatabase = ref('')
const catalogRoles = ref<string[]>([])
const selectedRole = ref('')
const tables = ref<TableMeta[]>([])
const tableSearch = ref('')
const selectedSchema = ref('public')
const selectedTable = ref<{ schema: string; name: string } | null>(null)
const tableTab = ref<TableTab>('structure')
const columns = ref<{ column: string; data_type: string; is_nullable: string }[]>([])
const indexes = ref<{ name: string; definition?: string }[]>([])
const dataPreview = ref<{ columns: string[]; rows: unknown[][]; row_count: number } | null>(null)

const sqlText = ref('SELECT 1')
const poolMode = ref<'read' | 'write'>('read')
const execResult = ref<{ columns: string[]; rows: unknown[][]; row_count: number; message?: string } | null>(null)
const execError = ref('')
const lastRunId = ref<string | null>(null)

const queriesTab = ref<QueriesTab>('recent')
const savedQueries = ref<{ id: number; title: string; sql: string; is_saved: boolean; last_run_at?: string }[]>([])
const runningQueries = ref<{ run_id: string; sql_snippet: string; started: string }[]>([])

const chatInput = ref('')
const chatLog = ref<{ role: 'user' | 'assistant'; text: string }[]>([])

const accountMenuOpen = ref(false)
const accountWrap = ref<HTMLElement | null>(null)
const createDbModalOpen = ref(false)
const newPhysicalDbName = ref('')
const createDbError = ref('')
const createDbBusy = ref(false)

const rowEditOpen = ref(false)
/** Snapshot of the row as loaded (column → value) for WHERE clause */
const rowEditOriginal = ref<Record<string, unknown>>({})
/** Editable string fields per column */
const rowEditFields = ref<Record<string, string>>({})
const rowEditError = ref('')
const rowEditBusy = ref(false)

const loginUsers = ref<{ name: string; host?: string }[]>([])
const newDbUsername = ref('')
const newDbPassword = ref('')
const newDbGrantRole = ref('')
const catalogErr = ref('')

function dbParams(): Record<string, string> {
  const o: Record<string, string> = {}
  if (selectedPhysicalDatabase.value) o.database = selectedPhysicalDatabase.value
  return o
}

const currentConnection = computed(() => connections.value.find((x) => x.id === selectedConnId.value) ?? null)

/** MySQL uses the physical database name as information_schema.table_schema, not "public". */
const effectiveSchema = computed(() => {
  const c = currentConnection.value
  if (!c) return selectedSchema.value
  if (c.driver === 'mysql') {
    const db = (selectedPhysicalDatabase.value || c.database || '').trim()
    return db || selectedSchema.value
  }
  return selectedSchema.value
})

const filteredTables = computed(() => {
  const q = tableSearch.value.trim().toLowerCase()
  if (!q) return tables.value
  return tables.value.filter((t) => `${t.schema}.${t.name}`.toLowerCase().includes(q))
})

const showLogout = computed(() => Boolean(auth.token || auth.user))

async function loadConnections() {
  const { data } = await http.get<{ connections: Connection[] }>('/api/connections')
  connections.value = data.connections
  if (connections.value.length === 0) {
    selectedConnId.value = null
    return
  }
  selectedConnId.value = connections.value[0].id
  const c = connections.value[0]
  selectedPhysicalDatabase.value = c.database || ''
}

async function loadDatabases() {
  if (!selectedConnId.value) return
  const { data } = await http.get<{ databases: string[] }>(
    `/api/connections/${selectedConnId.value}/databases`,
    { params: dbParams() },
  )
  databases.value = data.databases
}

async function loadCatalogRoles() {
  if (!selectedConnId.value) return
  catalogErr.value = ''
  try {
    const { data } = await http.get<{ roles: string[] }>(
      `/api/connections/${selectedConnId.value}/catalog/roles`,
      { params: dbParams() },
    )
    catalogRoles.value = data.roles || []
  } catch (e: unknown) {
    const err = e as { response?: { data?: { error?: string } } }
    catalogErr.value = err.response?.data?.error || 'Could not load roles'
    catalogRoles.value = []
  }
}

async function loadTables() {
  if (!selectedConnId.value) return
  const { data } = await http.get<{ tables: TableMeta[] }>(
    `/api/connections/${selectedConnId.value}/tables`,
    { params: { schema: effectiveSchema.value, ...dbParams() } },
  )
  tables.value = data.tables
}

async function loadColumns() {
  if (!selectedConnId.value || !selectedTable.value) return
  const { data } = await http.get<{ columns: typeof columns.value }>(
    `/api/connections/${selectedConnId.value}/columns`,
    { params: { schema: selectedTable.value.schema, table: selectedTable.value.name, ...dbParams() } },
  )
  columns.value = data.columns
}

async function loadIndexes() {
  if (!selectedConnId.value || !selectedTable.value) return
  const { data } = await http.get<{ indexes: typeof indexes.value }>(
    `/api/connections/${selectedConnId.value}/indexes`,
    { params: { schema: selectedTable.value.schema, table: selectedTable.value.name, ...dbParams() } },
  )
  indexes.value = data.indexes || []
}

async function loadRows() {
  if (!selectedConnId.value || !selectedTable.value) return
  const { data } = await http.get<{ result: typeof dataPreview.value }>(
    `/api/connections/${selectedConnId.value}/rows`,
    {
      params: {
        schema: selectedTable.value.schema,
        table: selectedTable.value.name,
        limit: 100,
        offset: 0,
        ...dbParams(),
      },
    },
  )
  dataPreview.value = data.result
}

async function loadLoginUsers() {
  if (!selectedConnId.value) return
  const { data } = await http.get<{ users: { name: string; host?: string }[] }>(
    `/api/connections/${selectedConnId.value}/catalog/login_users`,
    { params: dbParams() },
  )
  loginUsers.value = data.users || []
}

async function createCatalogUser() {
  catalogErr.value = ''
  if (!selectedConnId.value) return
  try {
    await http.post(`/api/connections/${selectedConnId.value}/catalog/users`, {
      username: newDbUsername.value,
      password: newDbPassword.value,
      role: newDbGrantRole.value,
    })
    newDbUsername.value = ''
    newDbPassword.value = ''
    await loadLoginUsers()
  } catch (e: unknown) {
    const err = e as { response?: { data?: { error?: string } } }
    catalogErr.value = err.response?.data?.error || 'Create user failed'
  }
}

async function loadQueries() {
  if (!selectedConnId.value) return
  if (queriesTab.value === 'running') {
    const { data } = await http.get<{ runs: typeof runningQueries.value }>(
      `/api/connections/${selectedConnId.value}/queries/running`,
      { params: dbParams() },
    )
    runningQueries.value = data.runs
    return
  }
  const saved = queriesTab.value === 'saved'
  const qparams: Record<string, string> = { ...dbParams() }
  if (saved) qparams.saved = '1'
  const { data } = await http.get<{ queries: typeof savedQueries.value }>(
    `/api/connections/${selectedConnId.value}/queries`,
    { params: qparams },
  )
  savedQueries.value = data.queries
}

async function runSql() {
  execError.value = ''
  execResult.value = null
  lastRunId.value = null
  if (!selectedConnId.value) return
  try {
    const { data } = await http.post(`/api/connections/${selectedConnId.value}/sql/execute`, {
      sql: sqlText.value,
      pool: auth.isEngineer ? poolMode.value : 'read',
      max_rows: 500,
      database: selectedPhysicalDatabase.value || undefined,
      role: selectedRole.value || undefined,
    })
    lastRunId.value = data.run_id as string
    execResult.value = data.result as typeof execResult.value
    await http.post(
      `/api/connections/${selectedConnId.value}/queries/touch_run`,
      { sql: sqlText.value },
      { params: dbParams() },
    )
  } catch (e: unknown) {
    const err = e as { response?: { data?: { error?: string } } }
    execError.value = err.response?.data?.error || 'Execute failed'
  }
}

async function cancelRun() {
  if (!lastRunId.value) return
  await http.post(`/api/connections/${selectedConnId.value}/sql/cancel`, { run_id: lastRunId.value })
}

async function sendChat() {
  if (!selectedConnId.value || !chatInput.value.trim()) return
  const q = chatInput.value.trim()
  chatLog.value.push({ role: 'user', text: q })
  chatInput.value = ''
  try {
    const { data } = await http.post<{ sql: string }>(
      `/api/connections/${selectedConnId.value}/ai/chat`,
      { message: q },
      { params: dbParams() },
    )
    chatLog.value.push({ role: 'assistant', text: data.sql })
    sqlText.value = data.sql
  } catch (e: unknown) {
    const err = e as { response?: { data?: { error?: string } } }
    chatLog.value.push({ role: 'assistant', text: 'Error: ' + (err.response?.data?.error || 'failed') })
  }
}

async function saveCurrentQuery() {
  if (!selectedConnId.value) return
  await http.post(
    `/api/connections/${selectedConnId.value}/queries`,
    { title: 'Saved query', sql: sqlText.value, is_saved: true },
    { params: dbParams() },
  )
  await loadQueries()
}

function openTable(t: TableMeta) {
  selectedTable.value = { schema: t.schema, name: t.name }
  tableTab.value = 'structure'
  nav.value = 'tables'
}

watch(selectedConnId, async () => {
  selectedTable.value = null
  databases.value = []
  tables.value = []
  catalogRoles.value = []
  selectedRole.value = ''
  if (selectedConnId.value) {
    const c = connections.value.find((x) => x.id === selectedConnId.value)
    if (c?.database) selectedPhysicalDatabase.value = c.database
    await loadDatabases()
    if (databases.value.length && !databases.value.includes(selectedPhysicalDatabase.value)) {
      selectedPhysicalDatabase.value = databases.value[0]
    }
    await loadTables()
    await loadCatalogRoles()
    await loadQueries()
  }
})

watch([selectedConnId, queriesTab], () => {
  void loadQueries()
})

watch([selectedConnId, selectedSchema], () => {
  void loadTables()
})

watch(selectedPhysicalDatabase, async () => {
  if (!selectedConnId.value) return
  await loadTables()
  await loadCatalogRoles()
  if (nav.value === 'users') await loadLoginUsers()
  if (selectedTable.value) {
    if (tableTab.value === 'structure') await loadColumns()
    if (tableTab.value === 'data') await loadRows()
    if (tableTab.value === 'indexes') await loadIndexes()
  }
})

watch([selectedTable, tableTab], async () => {
  if (!selectedTable.value) return
  if (tableTab.value === 'structure') await loadColumns()
  if (tableTab.value === 'data') await loadRows()
  if (tableTab.value === 'indexes') await loadIndexes()
})

watch(
  () => nav.value,
  (n) => {
    if (n === 'users') void loadLoginUsers()
  },
)

function logout() {
  accountMenuOpen.value = false
  auth.logout()
  void router.push('/login')
}

function toggleAccountMenu() {
  accountMenuOpen.value = !accountMenuOpen.value
}

function openCreateDbModal() {
  accountMenuOpen.value = false
  createDbModalOpen.value = true
  newPhysicalDbName.value = ''
  createDbError.value = ''
}

function closeCreateDbModal() {
  createDbModalOpen.value = false
  createDbError.value = ''
}

function buildCreateDatabaseSql(driver: string, name: string): string {
  const safe = name.replace(/[^A-Za-z0-9_]/g, '')
  if (driver.toLowerCase() === 'mysql') {
    return 'CREATE DATABASE `' + safe + '`'
  }
  return 'CREATE DATABASE "' + safe + '"'
}

async function submitCreateDatabase() {
  createDbError.value = ''
  if (!selectedConnId.value) return
  const raw = newPhysicalDbName.value.trim()
  if (!/^[A-Za-z0-9_]+$/.test(raw)) {
    createDbError.value = 'Use letters, digits, and underscores only.'
    return
  }
  const conn = connections.value.find((c) => c.id === selectedConnId.value)
  if (!conn) return
  createDbBusy.value = true
  try {
    await http.post(`/api/connections/${selectedConnId.value}/sql/execute`, {
      sql: buildCreateDatabaseSql(conn.driver, raw),
      pool: 'write',
      max_rows: 1,
      database: selectedPhysicalDatabase.value || undefined,
      role: selectedRole.value || undefined,
    })
    createDbModalOpen.value = false
    await loadDatabases()
    selectedPhysicalDatabase.value = raw
  } catch (e: unknown) {
    const err = e as { response?: { data?: { error?: string } } }
    createDbError.value = err.response?.data?.error || 'Could not create database'
  } finally {
    createDbBusy.value = false
  }
}

function onGlobalPointerDown(ev: PointerEvent) {
  const t = ev.target as Node
  if (accountMenuOpen.value && accountWrap.value && !accountWrap.value.contains(t)) {
    accountMenuOpen.value = false
  }
}

onMounted(async () => {
  document.addEventListener('pointerdown', onGlobalPointerDown, true)
  await loadConnections()
})

onUnmounted(() => {
  document.removeEventListener('pointerdown', onGlobalPointerDown, true)
})

function pickQuery(q: { sql: string }) {
  sqlText.value = q.sql
  nav.value = 'queries'
}

function columnMeta(name: string) {
  return columns.value.find((c) => c.column === name) ?? null
}

function isDateTimeColumn(name: string): boolean {
  const t = (columnMeta(name)?.data_type || '').toLowerCase()
  return /\b(datetime|timestamp)\b/.test(t)
}

function isIntegerColumn(name: string): boolean {
  const t = (columnMeta(name)?.data_type || '').toLowerCase()
  return /\b(tinyint|smallint|mediumint|int|integer|bigint)\b/.test(t)
}

function pad2(n: number): string {
  return String(n).padStart(2, '0')
}

function formatDateTimeLikeMysql(v: unknown): string {
  if (v === null || v === undefined) return ''
  const s = String(v).trim()
  if (!s) return ''
  // Already in "YYYY-MM-DD HH:mm:ss" (or without seconds)
  if (/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}(:\d{2})?$/.test(s)) {
    return s.length === 16 ? s + ':00' : s
  }
  const d = new Date(s)
  if (Number.isNaN(d.getTime())) return s
  // If the incoming string had an explicit timezone, prefer UTC to avoid local shifts.
  const hasTz = /z$/i.test(s) || /[+-]\d{2}:\d{2}$/.test(s)
  const y = hasTz ? d.getUTCFullYear() : d.getFullYear()
  const m = (hasTz ? d.getUTCMonth() : d.getMonth()) + 1
  const day = hasTz ? d.getUTCDate() : d.getDate()
  const hh = hasTz ? d.getUTCHours() : d.getHours()
  const mm = hasTz ? d.getUTCMinutes() : d.getMinutes()
  const ss = hasTz ? d.getUTCSeconds() : d.getSeconds()
  return `${y}-${pad2(m)}-${pad2(day)} ${pad2(hh)}:${pad2(mm)}:${pad2(ss)}`
}

function formatCellForInput(col: string, v: unknown): string {
  if (v === null || v === undefined) return ''
  if (isDateTimeColumn(col)) return formatDateTimeLikeMysql(v)
  return String(v)
}

function coerceEditedValue(raw: string, orig: unknown): unknown {
  const t = raw.trim()
  if (t === '') {
    return null
  }
  if (typeof orig === 'number' && Number.isFinite(orig)) {
    const n = Number(t)
    if (!Number.isNaN(n)) return n
    return t
  }
  if (typeof orig === 'boolean') {
    return t === 'true' || t === '1'
  }
  if (typeof orig === 'bigint') {
    try {
      return BigInt(t)
    } catch {
      return t
    }
  }
  return t
}

function normalizeForCompare(col: string, v: unknown): unknown {
  if (v === undefined) return null
  if (v === null) return null
  if (isDateTimeColumn(col)) return formatDateTimeLikeMysql(v)
  if (typeof v === 'number' && Number.isFinite(v)) return v
  if (typeof v === 'boolean') return v
  if (isIntegerColumn(col)) {
    const n = Number(String(v).trim())
    if (!Number.isNaN(n)) return n
  }
  return String(v)
}

function valuesEqual(a: unknown, b: unknown): boolean {
  if (a === null && b === null) return true
  if (a === null && b === undefined) return true
  if (a === undefined && b === null) return true
  if (typeof a === 'number' && typeof b === 'number') return Object.is(a, b)
  return String(a ?? '') === String(b ?? '')
}

const rowEditChangedValues = computed<Record<string, unknown>>(() => {
  const preview = dataPreview.value
  if (!preview) return {}
  const colsList = preview.columns
  const original = rowEditOriginal.value
  const changed: Record<string, unknown> = {}
  for (const c of colsList) {
    if (c === 'id') continue
    const raw = rowEditFields.value[c] ?? ''
    let edited = coerceEditedValue(raw, original[c])
    if (isDateTimeColumn(c) && edited !== null) edited = formatDateTimeLikeMysql(edited)
    const a = normalizeForCompare(c, edited)
    const b = normalizeForCompare(c, original[c])
    if (!valuesEqual(a, b)) changed[c] = edited
  }
  return changed
})

function quoteIdent(driver: string, name: string): string {
  if (driver.toLowerCase() === 'mysql') return '`' + name.replace(/`/g, '``') + '`'
  return '"' + name.replace(/"/g, '""') + '"'
}

function quoteLiteral(v: unknown): string {
  if (v === null || v === undefined) return 'NULL'
  if (typeof v === 'number' && Number.isFinite(v)) return String(v)
  if (typeof v === 'boolean') return v ? '1' : '0'
  const s = String(v)
  return "'" + s.replace(/'/g, "''") + "'"
}

const rowEditPreviewSql = computed(() => {
  const conn = currentConnection.value
  const t = selectedTable.value
  if (!conn || !t) return ''
  const changed = rowEditChangedValues.value
  const keys = Object.keys(changed)
  if (!keys.length) return '-- No changes'

  const q = (n: string) => quoteIdent(conn.driver, n)
  const tableExpr = conn.driver.toLowerCase() === 'mysql' ? `${q(t.schema)}.${q(t.name)}` : `${q(t.schema)}.${q(t.name)}`

  const setSql = keys.map((k) => `${q(k)} = ${quoteLiteral(changed[k])}`).join(', ')
  const whereSql = (() => {
    const idVal = rowEditOriginal.value['id']
    if (idVal === null || idVal === undefined || String(idVal).trim() === '') return '-- Missing id'
    return `${q('id')} = ${quoteLiteral(idVal)}`
  })()
  return `UPDATE ${tableExpr} SET ${setSql} WHERE ${whereSql};`
})

function closeRowEditor() {
  rowEditOpen.value = false
  rowEditError.value = ''
  rowEditOriginal.value = {}
  rowEditFields.value = {}
}

async function openRowEditor(rowIndex: number) {
  const preview = dataPreview.value
  if (!preview || !selectedTable.value || !selectedConnId.value) return
  const row = preview.rows[rowIndex]
  const cols = preview.columns
  const orig: Record<string, unknown> = {}
  const fields: Record<string, string> = {}
  cols.forEach((c, j) => {
    const v = row[j]
    orig[c] = v === undefined ? null : v
    fields[c] = formatCellForInput(c, v)
  })
  rowEditOriginal.value = orig
  rowEditFields.value = fields
  rowEditError.value = ''
  rowEditOpen.value = true
  if (!columns.value.length) {
    await loadColumns()
  }
}

async function submitRowUpdate() {
  rowEditError.value = ''
  if (!selectedConnId.value || !selectedTable.value || !dataPreview.value) return
  const original = { ...rowEditOriginal.value }
  const values = rowEditChangedValues.value
  if (!Object.keys(values).length) {
    closeRowEditor()
    return
  }
  rowEditBusy.value = true
  try {
    await http.post(
      `/api/connections/${selectedConnId.value}/rows/update`,
      {
        schema: selectedTable.value.schema,
        table: selectedTable.value.name,
        database: selectedPhysicalDatabase.value || undefined,
        role: selectedRole.value || undefined,
        original,
        values,
      },
      { params: dbParams() },
    )
    closeRowEditor()
    await loadRows()
  } catch (e: unknown) {
    const err = e as { response?: { data?: { error?: string } } }
    rowEditError.value = err.response?.data?.error || 'Update failed'
  } finally {
    rowEditBusy.value = false
  }
}
</script>

<template>
  <div class="app-shell">
    <header class="top-header">
      <div class="logo">ChatDB</div>
      <div class="header-right">
        <div v-if="selectedConnId" class="header-tools">
          <label>
            Database
            <select v-model="selectedPhysicalDatabase">
              <option v-for="d in databases" :key="d" :value="d">{{ d }}</option>
            </select>
          </label>
          <label>
            DB roles
            <select v-model="selectedRole">
              <option value="">(session default)</option>
              <option v-for="r in catalogRoles" :key="r" :value="r">{{ r }}</option>
            </select>
          </label>
          <label v-if="auth.isEngineer" class="pool">
            DB pool
            <select v-model="poolMode">
              <option value="read">Read</option>
              <option value="write">Write</option>
            </select>
          </label>
        </div>
        <div ref="accountWrap" class="account-slot">
          <button
            type="button"
            class="account-toggle"
            :class="{ 'is-open': accountMenuOpen }"
            :aria-expanded="accountMenuOpen"
            :aria-label="
              (accountMenuOpen ? 'Close account menu' : 'Open account menu') +
              (auth.user?.email ? ` (${auth.user.email})` : '')
            "
            aria-haspopup="menu"
            aria-controls="account-menu-dropdown"
            @click.stop="toggleAccountMenu"
          >
            <svg
              class="hamburger-icon"
              viewBox="0 0 24 24"
              width="22"
              height="22"
              fill="none"
              xmlns="http://www.w3.org/2000/svg"
              aria-hidden="true"
            >
              <path
                class="hamburger-line hamburger-line-top"
                d="M4 7h16"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
              />
              <path
                class="hamburger-line hamburger-line-mid"
                d="M4 12h16"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
              />
              <path
                class="hamburger-line hamburger-line-bot"
                d="M4 17h16"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
              />
            </svg>
          </button>
          <div
            id="account-menu-dropdown"
            v-show="accountMenuOpen"
            class="account-dropdown"
            role="menu"
            @click.stop
          >
            <div class="account-email">{{ auth.user?.email }}</div>
            <div v-if="auth.user?.role" class="account-role">{{ auth.user.role }}</div>
            <button type="button" class="dropdown-item" role="menuitem" @click="openCreateDbModal">Create new database</button>
            <button v-if="showLogout" type="button" class="dropdown-item linkish" role="menuitem" @click="logout">Log out</button>
          </div>
        </div>
      </div>
    </header>

    <div v-if="rowEditOpen" class="modal-backdrop" @click.self="closeRowEditor">
      <div class="modal modal-wide" role="dialog" aria-modal="true" aria-labelledby="row-edit-title" @click.stop>
        <div class="modal-header">
          <h2 id="row-edit-title" class="modal-title">Update row</h2>
          <button type="button" class="modal-close" aria-label="Close" @click="closeRowEditor">×</button>
        </div>
        <p v-if="selectedTable" class="muted small modal-sub">
          {{ selectedTable.schema }}.{{ selectedTable.name }}
        </p>
        <div class="modal-fields scroll">
          <label v-for="col in (dataPreview?.columns ?? []).filter((c) => c !== 'id')" :key="col" class="modal-field">
            {{ col }}
            <input
              v-model="rowEditFields[col]"
              :type="isIntegerColumn(col) ? 'number' : 'text'"
              :inputmode="isIntegerColumn(col) ? 'numeric' : undefined"
              autocomplete="off"
            />
          </label>
        </div>
        <div class="modal-preview">
          <div class="modal-preview-title muted small">Query preview</div>
          <pre class="mono preview-sql">{{ rowEditPreviewSql }}</pre>
        </div>
        <p v-if="rowEditError" class="error">{{ rowEditError }}</p>
        <div class="modal-actions">
          <button type="button" class="ghost" :disabled="rowEditBusy" @click="closeRowEditor">Cancel</button>
          <button type="button" class="primary" :disabled="rowEditBusy" @click="submitRowUpdate">
            {{ rowEditBusy ? 'Updating…' : 'Update' }}
          </button>
        </div>
      </div>
    </div>

    <div v-if="createDbModalOpen" class="modal-backdrop" @click.self="closeCreateDbModal">
      <div class="modal" role="dialog" aria-modal="true" aria-labelledby="create-db-title" @click.stop>
        <h2 id="create-db-title" class="modal-title">Create new database</h2>
        <p class="muted small">Runs on the server with write access. Name: letters, digits, underscores only.</p>
        <label class="modal-field">
          Database name
          <input v-model="newPhysicalDbName" type="text" autocomplete="off" pattern="[A-Za-z0-9_]+" />
        </label>
        <p v-if="createDbError" class="error">{{ createDbError }}</p>
        <div class="modal-actions">
          <button type="button" class="ghost" :disabled="createDbBusy" @click="closeCreateDbModal">Cancel</button>
          <button type="button" class="primary" :disabled="createDbBusy || !newPhysicalDbName.trim()" @click="submitCreateDatabase">
            {{ createDbBusy ? 'Creating…' : 'Create' }}
          </button>
        </div>
      </div>
    </div>

    <div class="layout">
    <aside class="left-rail">
      <div class="nav-btns">
        <button :class="{ on: nav === 'tables' }" type="button" @click="nav = 'tables'">Tables</button>
        <button :class="{ on: nav === 'queries' }" type="button" @click="nav = 'queries'">Queries</button>
        <button :class="{ on: nav === 'users' }" type="button" @click="nav = 'users'">Users</button>
        <button :class="{ on: nav === 'chat' }" type="button" @click="nav = 'chat'">Chat SQL</button>
      </div>
      <div class="table-block">
        <div class="table-block-title">Tables ({{ selectedSchema }})</div>
        <ul class="list table-scroll">
          <li v-for="t in filteredTables" :key="t.schema + '.' + t.name" @click="openTable(t)">
            <span class="kind">{{ t.kind === 'view' ? 'V' : 'T' }}</span> {{ t.schema }}.{{ t.name }}
          </li>
        </ul>
        <input v-model="tableSearch" class="search rail-search" type="search" placeholder="Search tables…" />
      </div>
    </aside>

    <main class="main">
      <div v-if="nav === 'tables'" class="panel browse">
        <div class="pane-wide">
          <template v-if="selectedTable">
            <h3>{{ selectedTable.schema }}.{{ selectedTable.name }}</h3>
            <div class="tabs">
              <button :class="{ on: tableTab === 'structure' }" type="button" @click="tableTab = 'structure'">
                Structure
              </button>
              <button :class="{ on: tableTab === 'data' }" type="button" @click="tableTab = 'data'">Data</button>
              <button :class="{ on: tableTab === 'indexes' }" type="button" @click="tableTab = 'indexes'">
                Indexes
              </button>
            </div>
            <div v-if="tableTab === 'structure'" class="scroll">
              <table class="grid">
                <thead>
                  <tr>
                    <th>Column</th>
                    <th>Type</th>
                    <th>Nullable</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="c in columns" :key="c.column">
                    <td>{{ c.column }}</td>
                    <td>{{ c.data_type }}</td>
                    <td>{{ c.is_nullable }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
            <div v-else-if="tableTab === 'data'" class="scroll">
              <table v-if="dataPreview" class="grid">
                <thead>
                  <tr>
                    <th v-for="col in dataPreview.columns" :key="col">{{ col }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr
                    v-for="(row, i) in dataPreview.rows"
                    :key="i"
                    class="data-row"
                    @click="openRowEditor(i)"
                  >
                    <td v-for="(cell, j) in row" :key="j">{{ cell }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
            <div v-else class="scroll">
              <table class="grid">
                <thead>
                  <tr>
                    <th>Index</th>
                    <th>Definition</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="ix in indexes" :key="ix.name">
                    <td>{{ ix.name }}</td>
                    <td class="mono def">{{ ix.definition || '—' }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </template>
          <p v-else class="muted">Select a table from the sidebar</p>
        </div>
      </div>

      <div v-else-if="nav === 'users'" class="panel users-panel">
        <div class="pane-wide">
          <h3>Database roles</h3>
          <p v-if="catalogErr" class="error">{{ catalogErr }}</p>
          <ul class="pill-list">
            <li v-for="r in catalogRoles" :key="r">{{ r }}</li>
          </ul>
          <hr class="sep" />
          <h3>Login users</h3>
          <ul class="list">
            <li v-for="(u, i) in loginUsers" :key="i">
              <strong>{{ u.name }}</strong>
              <span v-if="u.host" class="muted"> @ {{ u.host }}</span>
            </li>
          </ul>
          <h3 class="mt">New user</h3>
          <p class="muted small">
            Usernames and role names must be letters, digits, and underscores only. On MySQL, the role dropdown does not change the SQL session; it is used for GRANT when creating users.
          </p>
          <form class="user-form" @submit.prevent="createCatalogUser">
            <label>Username <input v-model="newDbUsername" required pattern="[A-Za-z0-9_]+" /></label>
            <label>Password <input v-model="newDbPassword" type="password" required /></label>
            <label>
              DB role
              <select v-model="newDbGrantRole">
                <option value="">(none)</option>
                <option v-for="r in catalogRoles" :key="'g-' + r" :value="r">{{ r }}</option>
              </select>
            </label>
            <button type="submit" class="primary">Create user</button>
          </form>
        </div>
      </div>

      <div v-else-if="nav === 'queries'" class="panel vertical">
        <div class="tabs qtabs">
          <button :class="{ on: queriesTab === 'saved' }" type="button" @click="queriesTab = 'saved'">Saved</button>
          <button :class="{ on: queriesTab === 'recent' }" type="button" @click="queriesTab = 'recent'">Recent</button>
          <button :class="{ on: queriesTab === 'running' }" type="button" @click="queriesTab = 'running'">Running</button>
        </div>
        <div class="editor-block">
          <textarea v-model="sqlText" class="sql" spellcheck="false" />
          <div class="actions">
            <button type="button" class="primary" @click="runSql">Run</button>
            <button type="button" class="ghost" :disabled="!lastRunId" @click="cancelRun">Cancel run</button>
            <button type="button" class="ghost" @click="saveCurrentQuery">Save</button>
          </div>
          <p v-if="execError" class="error">{{ execError }}</p>
          <div v-if="execResult" class="scroll result">
            <p v-if="execResult.message" class="muted">{{ execResult.message }}</p>
            <table v-else class="grid">
              <thead>
                <tr>
                  <th v-for="c in execResult.columns" :key="c">{{ c }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="(row, i) in execResult.rows" :key="i">
                  <td v-for="(cell, j) in row" :key="j">{{ cell }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
        <div class="side-list">
          <ul v-if="queriesTab !== 'running'" class="list scroll">
            <li v-for="q in savedQueries" :key="q.id" @click="pickQuery(q)">
              <strong>{{ q.title || 'untitled' }}</strong>
              <pre class="snippet">{{ q.sql.slice(0, 120) }}</pre>
            </li>
          </ul>
          <ul v-else class="list scroll">
            <li v-for="r in runningQueries" :key="r.run_id">
              <code>{{ r.run_id }}</code>
              <pre class="snippet">{{ r.sql_snippet }}</pre>
            </li>
          </ul>
        </div>
      </div>

      <div v-else class="panel hint">
        <p>Open the chat panel on the right to ask questions in natural language (schema-only AI).</p>
      </div>
    </main>

    <aside v-if="nav === 'chat' || chatPinned" class="right-rail">
      <header>
        <span>Chat</span>
        <button type="button" class="link" @click="chatPinned = !chatPinned">{{ chatPinned ? 'Unpin' : 'Pin' }}</button>
      </header>
      <div class="chat-log scroll">
        <div v-for="(m, i) in chatLog" :key="i" :class="['bubble', m.role]">
          {{ m.text }}
        </div>
      </div>
      <div class="chat-input">
        <textarea
          v-model="chatInput"
          rows="3"
          placeholder="Ask for a SELECT query…"
          @keydown.enter.exact.prevent="sendChat"
        />
        <button type="button" class="primary" @click="sendChat">Send</button>
      </div>
    </aside>

    </div>
  </div>
</template>

<style scoped>
.app-shell {
  display: flex;
  flex-direction: column;
  min-height: 100vh;
  background: #0d1117;
  color: #e6edf3;
}
.top-header {
  display: flex;
  align-items: center;
  gap: 1rem;
  flex-shrink: 0;
  padding: 0.5rem 1rem;
  border-bottom: 1px solid #30363d;
  background: #0b0e14;
  z-index: 10;
}
.logo {
  font-weight: 700;
  font-size: 1rem;
  letter-spacing: -0.02em;
}
.header-right {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 1rem;
  flex-wrap: wrap;
  justify-content: flex-end;
}
.header-tools {
  display: flex;
  align-items: center;
  gap: 1rem;
  flex-wrap: wrap;
}
.header-tools label {
  font-size: 0.8rem;
  color: #8b949e;
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
}
.account-slot {
  position: relative;
  flex-shrink: 0;
}
.account-toggle {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 2.25rem;
  height: 2.25rem;
  padding: 0;
  border-radius: 8px;
  border: 1px solid #30363d;
  background: #161b22;
  color: #e6edf3;
  cursor: pointer;
}
.account-toggle:hover {
  border-color: #484f58;
  color: #fff;
}
.account-toggle.is-open {
  border-color: #58a6ff;
  box-shadow: 0 0 0 1px #58a6ff33;
  color: #fff;
}
.hamburger-icon {
  display: block;
  pointer-events: none;
}
.hamburger-line {
  transform-origin: 12px 12px;
  transition:
    transform 0.2s ease,
    opacity 0.2s ease;
}
.account-toggle.is-open .hamburger-line-top {
  transform: translateY(5px) rotate(45deg);
}
.account-toggle.is-open .hamburger-line-mid {
  opacity: 0;
  transform: scaleX(0);
}
.account-toggle.is-open .hamburger-line-bot {
  transform: translateY(-5px) rotate(-45deg);
}
.account-dropdown {
  position: absolute;
  top: calc(100% + 6px);
  right: 0;
  min-width: 220px;
  padding: 0.65rem 0.75rem;
  border-radius: 8px;
  border: 1px solid #30363d;
  background: #0b0e14;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.45);
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  z-index: 30;
}
.account-email {
  font-size: 0.8rem;
  color: #e6edf3;
  word-break: break-all;
}
.account-role {
  font-size: 0.72rem;
  color: #8b949e;
  margin-bottom: 0.25rem;
}
.dropdown-item {
  text-align: left;
  padding: 0.35rem 0;
  border: none;
  background: none;
  color: #e6edf3;
  font-size: 0.8rem;
  cursor: pointer;
  border-radius: 4px;
}
.dropdown-item:hover {
  color: #58a6ff;
}
.dropdown-item.linkish {
  color: #58a6ff;
  padding-top: 0.5rem;
  margin-top: 0.25rem;
  border-top: 1px solid #30363d;
}
.modal-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(1, 4, 9, 0.72);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 50;
  padding: 1rem;
}
.modal {
  width: 100%;
  max-width: 400px;
  padding: 1.25rem;
  border-radius: 10px;
  border: 1px solid #30363d;
  background: #161b22;
  color: #e6edf3;
}
.modal-title {
  margin: 0 0 0.5rem;
  font-size: 1rem;
}
.modal-field {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  margin-top: 0.75rem;
  font-size: 0.8rem;
  color: #8b949e;
}
.modal-field input {
  padding: 0.45rem 0.5rem;
  border-radius: 6px;
  border: 1px solid #30363d;
  background: #0d1117;
  color: #e6edf3;
}
.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
  margin-top: 1rem;
}
.modal-wide {
  max-width: 520px;
  max-height: 90vh;
  display: flex;
  flex-direction: column;
}
.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 0.5rem;
}
.modal-header .modal-title {
  margin: 0;
  flex: 1;
}
.modal-sub {
  margin: 0.25rem 0 0;
}
.modal-close {
  flex-shrink: 0;
  width: 2rem;
  height: 2rem;
  padding: 0;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: #8b949e;
  font-size: 1.35rem;
  line-height: 1;
  cursor: pointer;
}
.modal-close:hover {
  color: #e6edf3;
  background: #21262d;
}
.modal-fields {
  max-height: min(60vh, 420px);
  overflow: auto;
  margin-top: 0.5rem;
  padding-right: 0.25rem;
}
.modal-preview {
  margin-top: 0.75rem;
  border-top: 1px solid #30363d;
  padding-top: 0.75rem;
}
.modal-preview-title {
  margin-bottom: 0.35rem;
}
.preview-sql {
  margin: 0;
  padding: 0.5rem;
  border-radius: 8px;
  border: 1px solid #30363d;
  background: #0d1117;
  color: #e6edf3;
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 160px;
  overflow: auto;
}
.data-row {
  cursor: pointer;
}
.data-row:hover {
  background: #21262d;
}
.layout {
  display: flex;
  flex: 1;
  min-height: 0;
  background: #0d1117;
  color: #e6edf3;
}
.left-rail {
  width: 240px;
  border-right: 1px solid #30363d;
  display: flex;
  flex-direction: column;
  padding: 0.75rem;
  gap: 0.5rem;
  background: #010409;
  min-height: 0;
}
.nav-btns {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}
.nav-btns button,
.left-rail .table-block .list li {
  text-align: left;
}
.nav-btns button {
  padding: 0.45rem 0.5rem;
  border-radius: 6px;
  border: 1px solid transparent;
  background: transparent;
  color: #e6edf3;
  cursor: pointer;
}
.nav-btns button.on {
  background: #1f6feb33;
  border-color: #1f6feb;
}
.table-block {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
  border-top: 1px solid #30363d;
  padding-top: 0.5rem;
  margin-top: 0.25rem;
}
.table-block-title {
  font-size: 0.8rem;
  color: #8b949e;
  margin-bottom: 0.35rem;
}
.table-scroll {
  flex: 1;
  overflow: auto;
  min-height: 80px;
}
.rail-search {
  margin-top: 0.5rem;
  flex-shrink: 0;
}
.link {
  background: none;
  border: none;
  color: #58a6ff;
  cursor: pointer;
  padding: 0;
  text-align: left;
}
.main {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
}
select,
input.search {
  padding: 0.35rem 0.5rem;
  border-radius: 6px;
  border: 1px solid #30363d;
  background: #161b22;
  color: #e6edf3;
}
.ghost {
  border: 1px solid #30363d;
  background: #21262d;
  color: #e6edf3;
  border-radius: 6px;
  padding: 0.35rem 0.75rem;
  cursor: pointer;
}
.primary {
  border: none;
  background: #238636;
  color: #fff;
  border-radius: 6px;
  padding: 0.45rem 1rem;
  cursor: pointer;
  font-weight: 600;
}
.panel {
  flex: 1;
  display: flex;
  min-height: 0;
}
.panel.browse,
.panel.users-panel {
  padding: 0.75rem 1rem;
}
.pane-wide {
  flex: 1;
  min-width: 0;
}
.panel.vertical {
  flex-direction: column;
}
h3 {
  margin: 0 0 0.5rem;
  font-size: 0.95rem;
}
.mt {
  margin-top: 1rem;
}
.sep {
  border: none;
  border-top: 1px solid #30363d;
  margin: 1rem 0;
}
.pill-list {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-wrap: wrap;
  gap: 0.35rem;
}
.pill-list li {
  font-size: 0.75rem;
  padding: 0.2rem 0.45rem;
  border-radius: 999px;
  background: #21262d;
  border: 1px solid #30363d;
}
.list {
  list-style: none;
  padding: 0;
  margin: 0;
}
.list li {
  padding: 0.35rem 0.25rem;
  border-radius: 4px;
  cursor: pointer;
  font-size: 0.85rem;
}
.table-block .list li:hover {
  background: #21262d;
}
.kind {
  color: #8b949e;
  margin-right: 0.25rem;
}
.scroll {
  max-height: calc(100vh - 200px);
  overflow: auto;
}
.mono {
  font-family: ui-monospace, monospace;
  font-size: 0.75rem;
}
.def {
  white-space: pre-wrap;
  word-break: break-word;
}
.grid {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.8rem;
}
.grid th,
.grid td {
  border: 1px solid #30363d;
  padding: 0.25rem 0.35rem;
  text-align: left;
}
.muted {
  color: #8b949e;
}
.small {
  font-size: 0.8rem;
}
.tabs {
  display: flex;
  gap: 0.35rem;
  margin-bottom: 0.5rem;
}
.tabs button {
  padding: 0.3rem 0.6rem;
  border-radius: 6px;
  border: 1px solid #30363d;
  background: #161b22;
  color: #e6edf3;
  cursor: pointer;
}
.tabs button.on {
  border-color: #58a6ff;
}
.qtabs {
  padding: 0.5rem 1rem 0;
}
.editor-block {
  flex: 1;
  display: flex;
  flex-direction: column;
  padding: 0.5rem 1rem;
  min-height: 0;
}
.sql {
  flex: 1;
  min-height: 140px;
  font-family: ui-monospace, monospace;
  font-size: 0.85rem;
  background: #161b22;
  color: #e6edf3;
  border: 1px solid #30363d;
  border-radius: 8px;
  padding: 0.5rem;
}
.actions {
  display: flex;
  gap: 0.5rem;
  margin: 0.5rem 0;
}
.error {
  color: #f85149;
  margin: 0.25rem 0;
}
.result {
  border: 1px solid #30363d;
  border-radius: 8px;
}
.side-list {
  height: 200px;
  border-top: 1px solid #30363d;
  padding: 0.5rem;
}
.snippet {
  margin: 0.15rem 0 0;
  font-size: 0.7rem;
  color: #8b949e;
  white-space: pre-wrap;
}
.user-form {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  max-width: 360px;
  margin-top: 0.5rem;
}
.user-form label {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
  font-size: 0.8rem;
  color: #8b949e;
}
.user-form input,
.user-form select {
  padding: 0.35rem;
  border-radius: 6px;
  border: 1px solid #30363d;
  background: #0d1117;
  color: #e6edf3;
}
.right-rail {
  width: 320px;
  border-left: 1px solid #30363d;
  display: flex;
  flex-direction: column;
  background: #010409;
}
.right-rail header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.5rem 0.75rem;
  border-bottom: 1px solid #30363d;
  font-weight: 600;
}
.chat-log {
  flex: 1;
  padding: 0.5rem;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}
.bubble {
  padding: 0.5rem;
  border-radius: 8px;
  font-size: 0.85rem;
  white-space: pre-wrap;
}
.bubble.user {
  background: #1f6feb33;
  align-self: flex-end;
}
.bubble.assistant {
  background: #21262d;
  align-self: flex-start;
}
.chat-input {
  padding: 0.5rem;
  border-top: 1px solid #30363d;
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}
.chat-input textarea {
  resize: vertical;
  background: #161b22;
  color: #e6edf3;
  border: 1px solid #30363d;
  border-radius: 6px;
  padding: 0.35rem;
}
.hint {
  padding: 1rem;
  color: #8b949e;
}
</style>
