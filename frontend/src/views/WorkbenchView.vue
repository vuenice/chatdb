<script setup lang="ts">
import { ref, watch, onMounted, onUnmounted, computed } from 'vue'
import { useRouter } from 'vue-router'
import { http } from '../api/http'
import { useAuthStore } from '../stores/auth'

const router = useRouter()
const auth = useAuthStore()

type Nav = 'tables' | 'queries' | 'users' | 'history'
type QueriesTab = 'chatsql' | 'saved' | 'running'
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
/** Display name for the active SQL "file" in the workbench header. */
const queryFileName = ref('untitled.sql')
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
/** Seconds, for results header (e.g. 0.43s) */
const lastRunSeconds = ref<number | null>(null)
const resultsFilter = ref('')

const sqlGutter = ref<HTMLElement | null>(null)
const sqlTextarea = ref<HTMLTextAreaElement | null>(null)

const queriesTab = ref<QueriesTab>('chatsql')
const savedQueries = ref<
  { id: number; title: string; sql: string; is_saved: boolean; last_run_at?: string; updated_at?: string }[]
>([])
const openSavedQueryMenuId = ref<number | null>(null)
/** Recent runs + saved, for the History (Conversations) panel */
const queryHistory = ref<typeof savedQueries.value>([])
const historySearch = ref('')
const selectedHistoryId = ref<number | null>(null)
const runningQueries = ref<{ run_id: string; sql_snippet: string; started: string }[]>([])

const chatInput = ref('')
const chatLog = ref<
  Array<{ role: 'user'; text: string } | { role: 'assistant'; text: string; suggestedSql?: string }>
>([])

const explainOpen = ref(false)
const explainText = ref('')
const explainError = ref('')
const explainLoading = ref(false)

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

const sqlLineNumbers = computed(() => {
  const n = (sqlText.value || '').split('\n').length
  return Array.from({ length: Math.max(1, n) }, (_, i) => i + 1)
})

const displayExecRows = computed(() => {
  const r = execResult.value
  if (!r || r.message) return []
  const q = resultsFilter.value.trim().toLowerCase()
  if (!q) return r.rows
  return r.rows.filter((row) => row.some((cell) => String(cell ?? '').toLowerCase().includes(q)))
})

function onSqlEditorScroll() {
  if (sqlGutter.value && sqlTextarea.value) {
    sqlGutter.value.scrollTop = sqlTextarea.value.scrollTop
  }
}

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
  return tables.value.filter((t) => `${t.name} ${t.schema}`.toLowerCase().includes(q))
})

const showLogout = computed(() => Boolean(auth.token || auth.user))

const selectedHistoryQuery = computed(
  () => queryHistory.value.find((q) => q.id === selectedHistoryId.value) ?? null,
)

function startOfLocalDay(d: Date): Date {
  const x = new Date(d)
  x.setHours(0, 0, 0, 0)
  return x
}

function historyTimeGroupLabel(d: Date, now: Date): string {
  const sod = startOfLocalDay(now)
  const dayMs = 864e5
  if (d >= sod) return 'Today'
  if (d >= new Date(sod.getTime() - dayMs)) return 'Yesterday'
  return 'Earlier'
}

const historyGroups = computed(() => {
  const q = historySearch.value.trim().toLowerCase()
  const now = new Date()
  const items = queryHistory.value
    .filter((row) => {
      if (!q) return true
      const t = (row.title || '').toLowerCase()
      return t.includes(q) || (row.sql || '').toLowerCase().includes(q)
    })
    .sort((a, b) => {
      const ta = a.last_run_at ? new Date(a.last_run_at).getTime() : 0
      const tb = b.last_run_at ? new Date(b.last_run_at).getTime() : 0
      return tb - ta
    })
  const orderLabels = ['Today', 'Yesterday', 'Earlier'] as const
  const byLabel = new Map<string, (typeof items)[number][]>()
  for (const l of orderLabels) byLabel.set(l, [])
  for (const row of items) {
    const d = row.last_run_at ? new Date(row.last_run_at) : new Date(0)
    const label = historyTimeGroupLabel(d, now)
    if (!byLabel.has(label)) byLabel.set(label, [])
    byLabel.get(label)!.push(row)
  }
  return orderLabels
    .filter((lab) => (byLabel.get(lab) ?? []).length > 0)
    .map((label) => ({ label, items: byLabel.get(label) ?? [] }))
})

function formatHistoryListTime(s?: string): string {
  if (!s) return ''
  const d = new Date(s)
  if (Number.isNaN(d.getTime())) return ''
  return d.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' })
}

function formatHistoryHeaderTime(s?: string): string {
  if (!s) return '—'
  const d = new Date(s)
  if (Number.isNaN(d.getTime())) return '—'
  return d.toLocaleString()
}

function queryKindTag(sql: string): string | null {
  const u = sql.toUpperCase()
  if (/\bJOIN\b/.test(u)) return 'join'
  if (/\bGROUP\s+BY\b/.test(u)) return 'aggregate'
  return null
}

const SQL_KEYWORDS = new Set(
  [
    'SELECT',
    'FROM',
    'WHERE',
    'JOIN',
    'INNER',
    'LEFT',
    'RIGHT',
    'FULL',
    'OUTER',
    'CROSS',
    'ON',
    'AND',
    'OR',
    'NOT',
    'AS',
    'WITH',
    'UNION',
    'ALL',
    'ORDER',
    'BY',
    'GROUP',
    'HAVING',
    'LIMIT',
    'OFFSET',
    'INSERT',
    'INTO',
    'VALUES',
    'UPDATE',
    'SET',
    'DELETE',
    'CREATE',
    'ALTER',
    'DROP',
    'TABLE',
    'INDEX',
    'CONSTRAINT',
    'PRIMARY',
    'KEY',
    'FOREIGN',
    'REFERENCES',
    'NULL',
    'IS',
    'IN',
    'LIKE',
    'BETWEEN',
    'CASE',
    'WHEN',
    'THEN',
    'ELSE',
    'END',
    'DISTINCT',
    'TRUE',
    'FALSE',
    'COUNT',
    'SUM',
    'AVG',
    'MIN',
    'MAX',
    'COALESCE',
    'CAST',
    'EXISTS',
    'RETURNING',
  ].map((k) => k.toUpperCase()),
)

function escapeHtml(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

function highlightSqlToHtml(src: string): string {
  const out: string[] = []
  const n = src.length
  let i = 0
  while (i < n) {
    if (src[i] === "'") {
      const start = i
      i++
      while (i < n) {
        if (src[i] === "'") {
          if (i + 1 < n && src[i + 1] === "'") {
            i += 2
            continue
          }
          i++
          break
        }
        i++
      }
      out.push(`<span class="sql-hl-str">${escapeHtml(src.slice(start, i))}</span>`)
      continue
    }
    if (src[i] === '"') {
      const start = i
      i++
      while (i < n) {
        if (src[i] === '\\' && i + 1 < n) {
          i += 2
          continue
        }
        if (src[i] === '"') {
          i++
          break
        }
        i++
      }
      out.push(`<span class="sql-hl-str">${escapeHtml(src.slice(start, i))}</span>`)
      continue
    }
    if (/\s/.test(src[i])) {
      out.push(escapeHtml(src[i]!))
      i++
      continue
    }
    if (/\d/.test(src[i]!)) {
      const start = i
      while (i < n && /[\d.]/.test(src[i]!)) i++
      out.push(`<span class="sql-hl-num">${escapeHtml(src.slice(start, i))}</span>`)
      continue
    }
    if (/[a-zA-Z_]/.test(src[i]!)) {
      const start = i
      i++
      while (i < n && /[a-zA-Z0-9_]/.test(src[i]!)) i++
      const w = src.slice(start, i)
      const up = w.toUpperCase()
      if (SQL_KEYWORDS.has(up)) {
        out.push(`<span class="sql-hl-kw">${escapeHtml(w)}</span>`)
      } else {
        out.push(`<span class="sql-hl-id">${escapeHtml(w)}</span>`)
      }
      continue
    }
    out.push(escapeHtml(src[i]!))
    i++
  }
  return out.join('')
}

function formatRelativeModified(iso?: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  const diffSec = (Date.now() - d.getTime()) / 1000
  if (diffSec < 45) return 'just now'
  if (diffSec < 3600) return `${Math.max(1, Math.floor(diffSec / 60))}m ago`
  if (diffSec < 86400) return `${Math.max(1, Math.floor(diffSec / 3600))}h ago`
  if (diffSec < 604800) return `${Math.max(1, Math.floor(diffSec / 86400))}d ago`
  if (diffSec < 2628000) return `${Math.max(1, Math.floor(diffSec / 604800))}w ago`
  return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}

function savedQueryPills(q: { sql: string }): string[] {
  const k = queryKindTag(q.sql)
  const a: string[] = [k ? k[0]!.toUpperCase() + k.slice(1) : 'Query', 'Saved']
  return a.slice(0, 2)
}

function sqlSnippetForCard(sql: string, maxLines = 5, maxChars = 420): string {
  const raw = (sql || '').replace(/\r\n/g, '\n')
  if (!raw.trim()) return ''
  const lines = raw.split('\n')
  const take = lines.slice(0, maxLines).join('\n')
  let out = take.length > maxChars ? take.slice(0, maxChars) : take
  if (take.length > maxChars) out += '…'
  else if (lines.length > maxLines) out += '\n…'
  return out
}

const savedDataSourceLabel = computed(() => {
  const c = currentConnection.value
  if (!c) return 'Connection'
  const name = (c.name || '').trim() || 'Connection'
  return name
})

function runSavedFromCard(q: (typeof savedQueries.value)[0]) {
  openSavedQueryMenuId.value = null
  pickQuery(q)
  void runSql()
}

async function deleteSavedQueryItem(q: (typeof savedQueries.value)[0]) {
  if (!selectedConnId.value) return
  try {
    await http.delete(`/api/connections/${selectedConnId.value}/queries/${q.id}`, { params: dbParams() })
    openSavedQueryMenuId.value = null
    await loadQueries()
  } catch {
    /* */
  }
}

function toggleSavedQueryMenu(id: number) {
  openSavedQueryMenuId.value = openSavedQueryMenuId.value === id ? null : id
}

type TableGroup = { letter: string; items: TableMeta[] }

const tableGroups = computed<TableGroup[]>(() => {
  const items = [...filteredTables.value].sort((a, b) => a.name.localeCompare(b.name))
  const byLetter = new Map<string, TableMeta[]>()
  for (const t of items) {
    const first = (t.name || '').trim().charAt(0).toUpperCase()
    const letter = /^[A-Z]$/.test(first) ? first : '#'
    if (!byLetter.has(letter)) byLetter.set(letter, [])
    byLetter.get(letter)!.push(t)
  }
  const letters = Array.from(byLetter.keys()).sort((a, b) => {
    if (a === '#') return 1
    if (b === '#') return -1
    return a.localeCompare(b)
  })
  return letters.map((l) => ({ letter: l, items: byLetter.get(l)! }))
})

function clearSelectedTable() {
  selectedTable.value = null
  columns.value = []
  indexes.value = []
  dataPreview.value = null
}

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
  if (queriesTab.value === 'chatsql') return
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
  const { data } = await http.get<{
    queries: (typeof savedQueries.value[0] & {
      LastRunAt?: string
      UpdatedAt?: string
    })[]
  }>(`/api/connections/${selectedConnId.value}/queries`, { params: qparams })
  savedQueries.value = (data.queries || []).map((r) => ({
    id: r.id,
    title: r.title,
    sql: r.sql,
    is_saved: r.is_saved,
    last_run_at: r.last_run_at ?? (r as { LastRunAt?: string }).LastRunAt,
    updated_at: r.updated_at ?? (r as { UpdatedAt?: string }).UpdatedAt,
  }))
}

async function loadQueryHistory() {
  if (!selectedConnId.value) return
  const { data } = await http.get<{ queries: typeof queryHistory.value }>(
    `/api/connections/${selectedConnId.value}/queries`,
    { params: { ...dbParams() } },
  )
  queryHistory.value = data.queries
  if (queryHistory.value.length) {
    const stillThere = queryHistory.value.some((q) => q.id === selectedHistoryId.value)
    if (selectedHistoryId.value == null || !stillThere) {
      selectedHistoryId.value = queryHistory.value[0].id
    }
  } else {
    selectedHistoryId.value = null
  }
}

async function runSql() {
  execError.value = ''
  execResult.value = null
  lastRunId.value = null
  lastRunSeconds.value = null
  resultsFilter.value = ''
  if (!selectedConnId.value) return
  const t0 = performance.now()
  try {
    const { data } = await http.post(`/api/connections/${selectedConnId.value}/sql/execute`, {
      sql: sqlText.value,
      pool: auth.isEngineer ? poolMode.value : 'read',
      max_rows: 500,
      database: selectedPhysicalDatabase.value || undefined,
      role: selectedRole.value || undefined,
    })
    lastRunSeconds.value = (performance.now() - t0) / 1000
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

function applySqlToEditor(sql: string) {
  sqlText.value = sql
}

function onChatKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    void sendChat()
  }
}

async function sendChat() {
  if (!selectedConnId.value || !chatInput.value.trim()) return
  const q = chatInput.value.trim()
  chatLog.value.push({ role: 'user', text: q })
  chatInput.value = ''
  try {
    const { data } = await http.post<{ sql?: string; error?: string }>(
      `/api/connections/${selectedConnId.value}/ai/chat`,
      { message: q },
      { params: dbParams() },
    )
    if (data.error) {
      chatLog.value.push({ role: 'assistant', text: data.error })
      return
    }
    const sql = (data.sql || '').trim()
    if (sql) {
      chatLog.value.push({
        role: 'assistant',
        text: 'Here is a query you can apply to the editor, then run when ready.',
        suggestedSql: sql,
      })
    } else {
      chatLog.value.push({ role: 'assistant', text: 'No SQL was returned for that message.' })
    }
  } catch (e: unknown) {
    const err = e as { response?: { data?: { error?: string } } }
    chatLog.value.push({ role: 'assistant', text: 'Error: ' + (err.response?.data?.error || 'failed') })
  }
}

async function runExplainForSql(sql: string) {
  explainError.value = ''
  explainText.value = ''
  if (!selectedConnId.value || !sql.trim()) return
  explainOpen.value = true
  explainLoading.value = true
  try {
    const { data } = await http.post<{ plan?: unknown; error?: string }>(
      `/api/connections/${selectedConnId.value}/sql/explain`,
      { sql },
      { params: dbParams() },
    )
    if (data.error) {
      explainError.value = data.error
      return
    }
    explainText.value =
      data.plan === null || data.plan === undefined
        ? ''
        : typeof data.plan === 'string'
          ? data.plan
          : JSON.stringify(data.plan, null, 2)
  } catch (e: unknown) {
    const err = e as { response?: { data?: { error?: string } } }
    explainError.value = err.response?.data?.error || 'EXPLAIN request failed'
  } finally {
    explainLoading.value = false
  }
}

function closeExplainModal() {
  explainOpen.value = false
  explainText.value = ''
  explainError.value = ''
}

function downloadResultsCsv() {
  const r = execResult.value
  if (!r || r.message || !r.columns.length) return
  const esc = (v: unknown) => {
    const s = String(v ?? '')
    if (/[",\n]/.test(s)) return `"${s.replace(/"/g, '""')}"`
    return s
  }
  const lines = [r.columns.map(esc).join(',')]
  for (const row of r.rows) {
    lines.push(row.map(esc).join(','))
  }
  const blob = new Blob([lines.join('\n')], { type: 'text/csv;charset=utf-8' })
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = (queryFileName.value || 'query').replace(/\.sql$/i, '') + '-results.csv'
  a.click()
  URL.revokeObjectURL(a.href)
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
    if (nav.value === 'history') await loadQueryHistory()
  }
})

watch([selectedConnId, queriesTab], () => {
  void loadQueries()
})

watch([selectedConnId, selectedSchema], () => {
  void loadTables()
})

watch(selectedPhysicalDatabase, async (next, prev) => {
  if (!selectedConnId.value) return
  // When the active database changes, take the user back to Tables so the UI
  // context matches the newly selected data source.
  if (next !== prev) {
    nav.value = 'tables'
    clearSelectedTable()
  }
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
    if (n === 'history') void loadQueryHistory()
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
  if (openSavedQueryMenuId.value !== null && t instanceof Element && !t.closest('.saved-query-menu')) {
    openSavedQueryMenuId.value = null
  }
}

onMounted(async () => {
  document.addEventListener('pointerdown', onGlobalPointerDown, true)
  await loadConnections()
})

onUnmounted(() => {
  document.removeEventListener('pointerdown', onGlobalPointerDown, true)
})

function pickQuery(q: { sql: string; title?: string }) {
  sqlText.value = q.sql
  nav.value = 'queries'
  queriesTab.value = 'chatsql'
  const t = (q.title || 'untitled').replace(/[^a-z0-9_.-]+/gi, '-').toLowerCase()
  queryFileName.value = t.endsWith('.sql') ? t : `${t}.sql`
}

function exportHistorySql() {
  const q = selectedHistoryQuery.value
  if (!q) return
  const base = (q.title || 'query').replace(/[^a-z0-9_.-]+/gi, '-').toLowerCase() || 'query'
  const blob = new Blob([q.sql], { type: 'text/sql;charset=utf-8' })
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = base.endsWith('.sql') ? base : `${base}.sql`
  a.click()
  URL.revokeObjectURL(a.href)
}

async function deleteHistoryQuery() {
  const q = selectedHistoryQuery.value
  if (!q || !selectedConnId.value) return
  try {
    await http.delete(`/api/connections/${selectedConnId.value}/queries/${q.id}`, { params: dbParams() })
    if (selectedHistoryId.value === q.id) selectedHistoryId.value = null
    await loadQueryHistory()
  } catch {
    /* surface via toast in future */
  }
}

function rerunFromHistory() {
  const q = selectedHistoryQuery.value
  if (!q) return
  pickQuery(q)
  void runSql()
}

function forkHistoryToWorkbench() {
  const q = selectedHistoryQuery.value
  if (!q) return
  pickQuery({ sql: q.sql, title: q.title })
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
              (auth.user?.username ? ` (${auth.user.username})` : '')
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
            <div class="account-username">{{ auth.user?.username }}</div>
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

    <div v-if="explainOpen" class="modal-backdrop" @click.self="closeExplainModal">
      <div class="modal modal-wide" role="dialog" aria-modal="true" aria-labelledby="explain-title" @click.stop>
        <div class="modal-header">
          <h2 id="explain-title" class="modal-title">Explain plan</h2>
          <button type="button" class="modal-close" aria-label="Close" @click="closeExplainModal">×</button>
        </div>
        <p v-if="explainLoading" class="muted small">Loading…</p>
        <p v-else-if="explainError" class="error">{{ explainError }}</p>
        <pre v-else-if="explainText" class="mono preview-sql explain-body">{{ explainText }}</pre>
        <p v-else class="muted small">No plan returned.</p>
        <div class="modal-actions">
          <button type="button" class="ghost" @click="closeExplainModal">Close</button>
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
    <aside class="left-rail" aria-label="Main navigation">
      <div class="nav-btns">
        <button
          :class="{ on: nav === 'queries' && queriesTab === 'chatsql' }"
          type="button"
          @click="nav = 'queries'; queriesTab = 'chatsql'"
        >
          Chat SQL
        </button>
        <button :class="{ on: nav === 'tables' }" type="button" @click="nav = 'tables'; clearSelectedTable()">
          Tables
        </button>
        <button :class="{ on: nav === 'history' }" type="button" @click="nav = 'history'">History</button>
        <button
          :class="{ on: nav === 'queries' && queriesTab !== 'chatsql' }"
          type="button"
          @click="nav = 'queries'; queriesTab = 'saved'"
        >
          Queries
        </button>
        <button :class="{ on: nav === 'users' }" type="button" @click="nav = 'users'">Users</button>
      </div>
    </aside>

    <main class="main">
      <div v-if="nav === 'tables'" class="panel browse">
        <div class="pane-wide">
          <template v-if="!selectedTable">
            <div class="tables-topbar">
              <input
                v-model="tableSearch"
                class="search tables-search"
                type="search"
                placeholder="Search tables…"
                autocomplete="off"
              />
            </div>

            <div class="tables-groups" role="list">
              <section v-for="g in tableGroups" :key="g.letter" class="tables-group" role="listitem">
                <div class="tables-letter">{{ g.letter }}</div>
                <div class="tables-items">
                  <button
                    v-for="t in g.items"
                    :key="t.schema + '.' + t.name"
                    type="button"
                    class="table-chip"
                    @click="openTable(t)"
                  >
                    <span class="table-chip-name">{{ t.name }}</span>
                    <span class="table-chip-kind">{{ t.kind === 'view' ? 'view' : 'table' }}</span>
                  </button>
                </div>
              </section>
            </div>
          </template>

          <template v-else>
            <div class="tables-detail-topbar">
              <button type="button" class="ghost" @click="clearSelectedTable">All tables</button>
              <div class="tables-detail-title">
                <h3>{{ selectedTable.schema }}.{{ selectedTable.name }}</h3>
              </div>
            </div>
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

      <div v-else-if="nav === 'history'" class="history-workspace">
        <aside class="history-list-pane" aria-label="Conversations">
          <div class="history-list-head">
            <h2 class="history-list-title">Conversations</h2>
            <input
              v-model="historySearch"
              class="search history-search"
              type="search"
              placeholder="Search logs, queries…"
              autocomplete="off"
            />
          </div>
          <div class="history-groups scroll">
            <p v-if="!queryHistory.length" class="muted small history-empty">No conversations yet. Run a query from Chat SQL.</p>
            <template v-else>
              <section v-for="g in historyGroups" :key="g.label" class="history-group">
                <div class="history-group-label">{{ g.label }}</div>
                <ul class="history-items">
                  <li v-for="q in g.items" :key="q.id">
                    <button
                      type="button"
                      class="history-item"
                      :class="{ 'is-active': q.id === selectedHistoryId }"
                      @click="selectedHistoryId = q.id"
                    >
                      <div class="history-item-title">
                        <strong>{{ q.title || 'Untitled' }}</strong>
                        <span class="history-item-time">{{ formatHistoryListTime(q.last_run_at) }}</span>
                      </div>
                      <p class="history-item-snippet">{{ (q.sql || '').replace(/\s+/g, ' ').trim().slice(0, 100) }}{{ (q.sql || '').length > 100 ? '…' : '' }}</p>
                      <span v-if="queryKindTag(q.sql)" class="history-tag">{{ queryKindTag(q.sql) }}</span>
                    </button>
                  </li>
                </ul>
              </section>
            </template>
          </div>
        </aside>

        <div class="history-detail-pane">
          <template v-if="selectedHistoryQuery">
            <div class="history-detail-top">
              <div class="history-detail-titles">
                <h1 class="history-detail-h1">{{ selectedHistoryQuery.title || 'Untitled' }}</h1>
                <p class="history-detail-meta muted small">
                  {{ formatHistoryHeaderTime(selectedHistoryQuery.last_run_at) }}
                  <span class="history-detail-sep">·</span>
                  <span class="history-detail-db">{{ selectedPhysicalDatabase || currentConnection?.database || '—' }}</span>
                </p>
              </div>
              <div class="history-detail-actions">
                <button type="button" class="ghost history-action-btn" @click="rerunFromHistory">Re-run</button>
                <button type="button" class="ghost history-action-btn" @click="exportHistorySql">Export</button>
                <button
                  type="button"
                  class="ghost history-action-btn history-trash"
                  title="Delete"
                  aria-label="Delete conversation"
                  @click="deleteHistoryQuery"
                >
                  <svg viewBox="0 0 24 24" width="18" height="18" fill="none" aria-hidden="true">
                    <path
                      d="M6 7h12M9 7V5a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2M10 11v6m4-6v6M5 7l1 14a1 1 0 0 0 1 1h10a1 1 0 0 0 1-1l1-14"
                      stroke="currentColor"
                      stroke-width="1.5"
                      stroke-linecap="round"
                    />
                  </svg>
                </button>
              </div>
            </div>
            <div class="history-prompt">
              <span class="history-prompt-label">Request</span>
              <p class="history-prompt-text">{{ (selectedHistoryQuery.title || 'SQL session').trim() || '—' }}</p>
            </div>
            <div class="history-sql-block">
              <div class="history-sql-head">
                <span>SQL</span>
              </div>
              <pre class="history-sql-body mono">{{ selectedHistoryQuery.sql }}</pre>
            </div>
            <a class="history-exec-hint linkish" href="#" @click.prevent="forkHistoryToWorkbench">Execution result · open in workbench to re-run and preview</a>
            <div class="history-historical-bar">
              <span class="history-historical-note">This is a historical view</span>
              <button type="button" class="primary history-fork" @click="forkHistoryToWorkbench">Fork session</button>
            </div>
          </template>
          <p v-else class="history-detail-placeholder muted">Select a conversation to view details.</p>
        </div>
      </div>

      <div v-else-if="nav === 'queries'" class="queries-workspace">
        <div class="panel vertical queries-panel-inner">
          <template v-if="queriesTab === 'chatsql'">
            <div class="chatsql-split">
              <div class="chatsql-main">
                <div class="sql-editor-split">
                  <div class="sql-editor-pane">
                    <div class="sql-gutter-wrap">
                      <div ref="sqlGutter" class="sql-gutter mono" aria-hidden="true">
                        <div v-for="n in sqlLineNumbers" :key="n" class="sql-gutter-line">{{ n }}</div>
                      </div>
                      <textarea
                        ref="sqlTextarea"
                        v-model="sqlText"
                        class="sql sql-input"
                        spellcheck="false"
                        @scroll="onSqlEditorScroll"
                      />
                    </div>
                    <div class="sql-workbench-actions sql-editor-foot" role="toolbar" aria-label="Query actions">
                      <button type="button" class="ghost sql-btn-icon" title="Save query" @click="saveCurrentQuery">
                        <svg viewBox="0 0 24 24" width="18" height="18" fill="none" aria-hidden="true">
                          <path
                            d="M19 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11l5 5v11a2 2 0 0 1-2 2z"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linejoin="round"
                          />
                          <path d="M17 21v-8H7v8" stroke="currentColor" stroke-width="2" />
                          <path d="M7 3v5h8" stroke="currentColor" stroke-width="2" />
                        </svg>
                        <span class="sql-btn-label">Save</span>
                      </button>
                      <button type="button" class="primary run-query-btn" @click="runSql">
                        <svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor" aria-hidden="true">
                          <path d="M8 5v14l11-7z" />
                        </svg>
                        <span>Run query</span>
                      </button>
                      <button type="button" class="ghost sql-btn-compact" :disabled="!lastRunId" @click="cancelRun">
                        Cancel run
                      </button>
                    </div>
                  </div>

                  <div class="sql-results-stack">
                    <div class="sql-results-head">
                      <div class="sql-results-title">
                        <span class="sql-results-heading">Query results</span>
                        <span v-if="execResult && !execResult.message" class="muted small sql-results-meta">
                          <template v-if="resultsFilter.trim()">
                            {{ displayExecRows.length }} of {{ execResult.rows.length }} rows
                          </template>
                          <template v-else> {{ execResult.row_count }} rows </template>
                          <template v-if="lastRunSeconds !== null">· {{ lastRunSeconds.toFixed(2) }}s</template>
                        </span>
                      </div>
                      <div v-if="execResult && !execResult.message" class="sql-results-tools">
                        <input
                          v-model="resultsFilter"
                          type="search"
                          class="results-filter-input"
                          placeholder="Filter rows…"
                          aria-label="Filter result rows"
                          autocomplete="off"
                        />
                        <button
                          type="button"
                          class="ghost icon-only"
                          title="Download CSV"
                          @click="downloadResultsCsv"
                        >
                          <svg viewBox="0 0 24 24" width="18" height="18" fill="none" aria-hidden="true">
                            <path
                              d="M12 3v12m0 0l4-4m-4 4L8 11M4 21h16"
                              stroke="currentColor"
                              stroke-width="2"
                              stroke-linecap="round"
                              stroke-linejoin="round"
                            />
                          </svg>
                        </button>
                      </div>
                    </div>
                    <p v-if="execError" class="error sql-results-error">{{ execError }}</p>
                    <div v-else-if="execResult" class="sql-results-body scroll">
                      <p v-if="execResult.message" class="muted">{{ execResult.message }}</p>
                      <table v-else class="grid grid-striped">
                        <thead>
                          <tr>
                            <th v-for="c in execResult.columns" :key="c">{{ c }}</th>
                          </tr>
                        </thead>
                        <tbody>
                          <tr v-for="(row, i) in displayExecRows" :key="i">
                            <td v-for="(cell, j) in row" :key="j">{{ cell }}</td>
                          </tr>
                        </tbody>
                      </table>
                    </div>
                    <div v-else class="sql-results-placeholder muted small">Run a query to see results here.</div>
                  </div>
                </div>
              </div>

              <aside class="ai-panel" aria-label="SQL assistant">
                <div class="ai-panel-header">
                  <span class="ai-panel-title">ChatDB AI</span>
                </div>
                <div class="ai-messages scroll">
                  <p v-if="!chatLog.length" class="muted small ai-empty">
                    Ask for a query in natural language. Suggested SQL appears here with actions to apply or explain.
                  </p>
                  <div
                    v-for="(m, i) in chatLog"
                    :key="i"
                    :class="['ai-msg', m.role === 'user' ? 'ai-msg-user' : 'ai-msg-assistant']"
                  >
                    <div v-if="m.role === 'user'" class="ai-bubble ai-bubble-user">{{ m.text }}</div>
                    <div v-else class="ai-bubble ai-bubble-assistant">
                      <p class="ai-assist-text">{{ m.text }}</p>
                      <template v-if="m.suggestedSql">
                        <pre class="ai-sql-block mono">{{ m.suggestedSql }}</pre>
                        <div class="ai-assist-actions">
                          <button type="button" class="apply-sql-btn" @click="applySqlToEditor(m.suggestedSql!)">
                            Apply to editor
                          </button>
                          <button type="button" class="linkish-btn" @click="runExplainForSql(m.suggestedSql!)">
                            Explain plan
                          </button>
                        </div>
                      </template>
                    </div>
                  </div>
                </div>
                <div class="ai-composer">
                  <textarea
                    v-model="chatInput"
                    class="ai-composer-input"
                    rows="3"
                    placeholder="Ask ChatDB to generate or debug SQL…"
                    autocomplete="off"
                    @keydown="onChatKeydown"
                  />
                  <div class="ai-composer-row">
                    <span class="muted tiny">Press Enter to send · Shift+Enter for new line</span>
                    <button type="button" class="primary ai-send" title="Send" @click="sendChat">
                      <svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor" aria-hidden="true">
                        <path d="M2 21l21-9L2 3v7l15 2-15 2v7z" />
                      </svg>
                    </button>
                  </div>
                </div>
              </aside>
            </div>
          </template>
          <template v-else>
            <div class="tabs qtabs" :class="{ 'qtabs-saved': queriesTab === 'saved' }">
              <button :class="{ on: queriesTab === 'saved' }" type="button" @click="queriesTab = 'saved'">Saved</button>
              <button :class="{ on: queriesTab === 'running' }" type="button" @click="queriesTab = 'running'">Running</button>
            </div>
            <div
              class="side-list full"
              :class="{ 'is-saved-grid': queriesTab === 'saved' }"
            >
              <ul v-if="queriesTab === 'running'" class="list scroll list-fill">
                <li v-for="r in runningQueries" :key="r.run_id">
                  <code>{{ r.run_id }}</code>
                  <pre class="snippet">{{ r.sql_snippet }}</pre>
                </li>
              </ul>
              <div v-else class="saved-queries-layer scroll list-fill">
                <p v-if="!savedQueries.length" class="saved-queries-empty">
                  No saved queries yet. Run SQL in Chat SQL, then use Save in the header.
                </p>
                <div v-else class="saved-queries-grid">
                  <article
                    v-for="q in savedQueries"
                    :key="q.id"
                    class="saved-query-card"
                    @click="pickQuery(q)"
                  >
                    <div class="saved-query-card-hd">
                      <div class="saved-query-titles">
                        <h3 class="saved-query-title">{{ q.title || 'Untitled' }}</h3>
                        <p class="saved-query-meta">
                          {{ savedDataSourceLabel }} · Modified {{ formatRelativeModified(q.updated_at || q.last_run_at) }}
                        </p>
                      </div>
                      <div class="saved-query-menu" @click.stop>
                        <button
                          type="button"
                          class="saved-query-dots"
                          :aria-expanded="openSavedQueryMenuId === q.id"
                          aria-label="Query actions"
                          @click="toggleSavedQueryMenu(q.id)"
                        >
                          <svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor" aria-hidden="true">
                            <path
                              d="M12 5a1.5 1.5 0 1 1 0-3 1.5 1.5 0 0 1 0 3zm0 8.5a1.5 1.5 0 1 1 0-3 1.5 1.5 0 0 1 0 3zm0 8.5a1.5 1.5 0 1 1 0-3 1.5 1.5 0 0 1 0 3z"
                            />
                          </svg>
                        </button>
                        <div v-if="openSavedQueryMenuId === q.id" class="saved-query-menu-dd" role="menu">
                          <button type="button" role="menuitem" @click.stop="deleteSavedQueryItem(q)">Delete</button>
                        </div>
                      </div>
                    </div>
                    <div
                      class="saved-query-code mono"
                      v-html="highlightSqlToHtml(sqlSnippetForCard(q.sql))"
                    />
                    <div class="saved-query-foot">
                      <div class="saved-query-tags">
                        <span v-for="tag in savedQueryPills(q)" :key="tag" class="saved-tag">{{ tag }}</span>
                      </div>
                      <button
                        type="button"
                        class="saved-query-run"
                        title="Run in workbench"
                        aria-label="Run query"
                        @click.stop="runSavedFromCard(q)"
                      >
                        <svg viewBox="0 0 24 24" width="20" height="20" fill="currentColor" aria-hidden="true">
                          <path d="M8 5v14l11-7z" />
                        </svg>
                      </button>
                    </div>
                  </article>
                </div>
              </div>
            </div>
          </template>
        </div>
      </div>
    </main>

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
.account-username {
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
.left-rail .list li {
  text-align: left;
}
.nav-btns button {
  padding: 0.45rem 0.5rem;
  border-radius: 6px;
  border: 1px solid transparent;
  background: transparent;
  color: #e6edf3;
  cursor: pointer;
  font-size: 0.85rem;
}
.nav-btns button.on {
  background: #1f6feb33;
  border-color: #1f6feb;
}
.nav-btns button:hover:not(.on) {
  background: #161b22;
}
.history-workspace {
  flex: 1;
  display: flex;
  min-height: 0;
  min-width: 0;
  align-items: stretch;
}
.history-list-pane {
  width: min(100%, 320px);
  min-width: 220px;
  max-width: 36%;
  border-right: 1px solid #30363d;
  background: #0b0e14;
  display: flex;
  flex-direction: column;
  min-height: 0;
}
.history-list-head {
  padding: 0.75rem 0.9rem 0.55rem;
  border-bottom: 1px solid #21262d;
  flex-shrink: 0;
}
.history-list-title {
  margin: 0 0 0.5rem;
  font-size: 0.95rem;
  font-weight: 600;
  color: #e6edf3;
  letter-spacing: -0.02em;
}
.history-search {
  width: 100%;
  font-size: 0.8rem;
  padding: 0.4rem 0.5rem;
  border-radius: 6px;
  border: 1px solid #30363d;
  background: #0d1117;
  color: #e6edf3;
}
.history-groups {
  flex: 1;
  min-height: 0;
  padding: 0.4rem 0.45rem 0.75rem;
}
.history-groups.scroll {
  max-height: none;
}
.history-empty {
  margin: 0.75rem 0.5rem;
  line-height: 1.4;
}
.history-group {
  margin-top: 0.5rem;
}
.history-group-label {
  font-size: 0.62rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.1em;
  color: #6e7681;
  padding: 0.3rem 0.4rem 0.35rem;
}
.history-items {
  list-style: none;
  margin: 0;
  padding: 0;
}
.history-item {
  display: block;
  width: 100%;
  text-align: left;
  background: transparent;
  border: 1px solid transparent;
  border-radius: 6px;
  padding: 0.45rem 0.5rem 0.5rem 0.55rem;
  margin-bottom: 0.15rem;
  cursor: pointer;
  color: #e6edf3;
  position: relative;
  transition: background 0.12s ease;
}
.history-item strong {
  font-size: 0.82rem;
  font-weight: 600;
}
.history-item.is-active {
  background: #161b22;
  border-color: #30363d;
  box-shadow: inset 3px 0 0 #1f6feb;
}
.history-item:hover:not(.is-active) {
  background: #161b22aa;
  border-color: #21262d;
}
.history-item-title {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 0.4rem;
}
.history-item-time {
  flex-shrink: 0;
  font-size: 0.7rem;
  color: #6e7681;
}
.history-item-snippet {
  margin: 0.25rem 0 0;
  font-size: 0.72rem;
  color: #8b949e;
  line-height: 1.35;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.history-tag {
  display: inline-block;
  margin-top: 0.4rem;
  font-size: 0.6rem;
  font-weight: 600;
  text-transform: lowercase;
  padding: 0.12rem 0.4rem;
  border-radius: 4px;
  background: #23863622;
  color: #3fb950;
  border: 1px solid #2ea04355;
  letter-spacing: 0.02em;
}
.history-detail-pane {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  background: #0d1117;
  padding: 0.6rem 1rem 0;
  min-height: 0;
  overflow: auto;
}
.history-detail-top {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.5rem 1rem;
  margin-bottom: 0.75rem;
  padding-bottom: 0.65rem;
  border-bottom: 1px solid #30363d;
}
.history-detail-h1 {
  margin: 0 0 0.25rem;
  font-size: 1.05rem;
  font-weight: 600;
  line-height: 1.2;
  color: #e6edf3;
}
.history-detail-meta {
  margin: 0;
  font-size: 0.78rem;
}
.history-detail-sep {
  margin: 0 0.35rem;
  color: #484f58;
}
.history-detail-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.35rem;
}
.history-action-btn {
  font-size: 0.78rem;
  padding: 0.3rem 0.65rem;
}
.history-trash {
  padding: 0.3rem 0.45rem;
  color: #8b949e;
}
.history-trash:hover {
  color: #f85149;
  border-color: #f8514955;
}
.history-prompt {
  border: 1px solid #30363d;
  border-radius: 8px;
  background: #0b0e14;
  padding: 0.5rem 0.75rem 0.6rem;
  margin-bottom: 0.85rem;
}
.history-prompt-label {
  display: block;
  font-size: 0.62rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: #6e7681;
  margin-bottom: 0.35rem;
}
.history-prompt-text {
  margin: 0;
  font-size: 0.85rem;
  line-height: 1.45;
  color: #c9d1d9;
}
.history-sql-block {
  border: 1px solid #30363d;
  border-radius: 8px;
  overflow: hidden;
  background: #161b22;
  margin-bottom: 0.75rem;
}
.history-sql-head {
  padding: 0.35rem 0.6rem;
  font-size: 0.7rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: #8b949e;
  border-bottom: 1px solid #21262d;
  background: #0b0e14;
}
.history-sql-body {
  margin: 0;
  padding: 0.6rem 0.75rem 0.75rem;
  font-size: 0.8rem;
  line-height: 1.5;
  color: #79c0ff;
  white-space: pre-wrap;
  word-break: break-word;
  max-height: min(50vh, 400px);
  overflow: auto;
  background: #0d1117;
}
.history-exec-hint {
  display: inline-block;
  font-size: 0.78rem;
  color: #58a6ff;
  text-decoration: none;
  margin-bottom: 0.75rem;
  cursor: pointer;
}
.history-exec-hint:hover {
  text-decoration: underline;
}
.history-historical-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  margin-top: auto;
  padding: 0.5rem 0;
  border-top: 1px solid #21262d;
  font-size: 0.75rem;
  color: #8b949e;
  position: sticky;
  bottom: 0;
  background: linear-gradient(180deg, transparent, #0d1117 20%);
  padding-top: 0.75rem;
  margin-bottom: 0.5rem;
}
.history-historical-note {
  flex: 1;
  min-width: 8rem;
}
.history-fork {
  font-size: 0.75rem;
  padding: 0.4rem 0.9rem;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
.history-detail-placeholder {
  margin: 2rem 1rem;
  text-align: center;
  font-size: 0.9rem;
}
@media (max-width: 900px) {
  .history-workspace {
    flex-direction: column;
  }
  .history-list-pane {
    width: 100%;
    max-width: none;
    min-height: 200px;
    border-right: none;
    border-bottom: 1px solid #30363d;
  }
}
.tables-topbar {
  display: flex;
  gap: 0.75rem;
  align-items: center;
  margin-bottom: 0.75rem;
}
.tables-search {
  width: 100%;
  max-width: 520px;
}
.tables-groups {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 0.85rem 1rem;
  align-items: start;
}
.tables-group {
  border: 1px solid #30363d;
  border-radius: 10px;
  background: #0b0e14;
  padding: 0.75rem;
  min-width: 0;
}
.tables-letter {
  font-size: 1.35rem;
  font-weight: 800;
  letter-spacing: -0.02em;
  line-height: 1;
  margin-bottom: 0.5rem;
  color: #e6edf3;
}
.tables-items {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
  gap: 0.35rem 0.45rem;
}
.table-chip {
  text-align: left;
  border: 1px solid #30363d;
  background: #161b22;
  color: #e6edf3;
  border-radius: 8px;
  padding: 0.4rem 0.5rem;
  cursor: pointer;
  min-width: 0;
}
.table-chip:hover {
  border-color: #58a6ff;
  box-shadow: 0 0 0 1px #58a6ff22;
}
.table-chip-name {
  display: block;
  font-size: 0.85rem;
  line-height: 1.15;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.table-chip-kind {
  display: block;
  font-size: 0.7rem;
  color: #8b949e;
  margin-top: 0.15rem;
  text-transform: uppercase;
  letter-spacing: 0.06em;
}
.tables-detail-topbar {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin-bottom: 0.35rem;
}
.tables-detail-title {
  min-width: 0;
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
  min-height: 0;
}
.queries-workspace {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
}
.queries-panel-inner {
  min-height: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
}
.chatsql-split {
  flex: 1;
  display: flex;
  min-height: 0;
  min-width: 0;
  align-items: stretch;
}
.chatsql-main {
  flex: 1 1 62%;
  min-width: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  background: #0b0e14;
  border-right: 1px solid #30363d;
}
.sql-workbench-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.4rem;
}
.sql-editor-foot {
  flex-shrink: 0;
  margin-top: 0.5rem;
  padding-top: 0.5rem;
  border-top: 1px solid #21262d;
}
.run-query-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.4rem 0.9rem;
}
.sql-btn-icon {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
}
.sql-btn-label {
  font-size: 0.8rem;
}
.sql-btn-compact {
  font-size: 0.75rem;
  padding: 0.3rem 0.5rem;
}
.sql-editor-split {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
}
.sql-editor-pane {
  flex: 1 1 48%;
  min-height: 140px;
  padding: 0.65rem 0.9rem 0.5rem;
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.sql-gutter-wrap {
  flex: 1;
  display: flex;
  min-height: 0;
  min-width: 0;
  border-radius: 8px;
  border: 1px solid #30363d;
  background: #161b22;
  overflow: hidden;
}
.sql-gutter {
  flex-shrink: 0;
  width: 2.5rem;
  padding: 0.5rem 0.4rem 0.5rem 0.5rem;
  overflow: hidden;
  text-align: right;
  color: #6e7681;
  font-size: 0.85rem;
  line-height: 1.5;
  user-select: none;
  background: #0b0e14;
  border-right: 1px solid #21262d;
}
.sql-gutter-line {
  min-height: 1.5em;
}
.sql-input {
  flex: 1;
  min-width: 0;
  min-height: 0;
  margin: 0;
  border: none;
  border-radius: 0;
  resize: none;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 0.85rem;
  line-height: 1.5;
  padding: 0.5rem 0.55rem;
  background: #161b22;
  color: #e6edf3;
  white-space: pre;
  overflow: auto;
  tab-size: 2;
}
.sql-input:focus {
  outline: none;
  box-shadow: inset 0 0 0 1px #58a6ff;
}
.sql-results-stack {
  flex: 1 1 45%;
  min-height: 160px;
  display: flex;
  flex-direction: column;
  min-width: 0;
  border-top: 1px solid #30363d;
  background: #0b0e14;
}
.sql-results-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 0.4rem 0.75rem;
  padding: 0.45rem 0.9rem;
  border-bottom: 1px solid #21262d;
  background: #0b0e14;
}
.sql-results-title {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 0.4rem 0.75rem;
  min-width: 0;
}
.sql-results-heading {
  font-size: 0.78rem;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: #8b949e;
}
.sql-results-meta {
  font-size: 0.78rem;
}
.sql-results-tools {
  display: flex;
  align-items: center;
  gap: 0.35rem;
}
.results-filter-input {
  width: 8rem;
  min-width: 0;
  padding: 0.2rem 0.4rem;
  font-size: 0.75rem;
  border-radius: 4px;
  border: 1px solid #30363d;
  background: #161b22;
  color: #e6edf3;
}
.sql-results-error {
  margin: 0.4rem 0.9rem 0;
}
.sql-results-body {
  flex: 1;
  min-height: 0;
  max-height: none;
  padding: 0.35rem 0.5rem 0.6rem 0.9rem;
  overflow: auto;
}
.sql-results-body.scroll {
  max-height: none;
}
.sql-results-placeholder {
  padding: 1rem 0.9rem;
}
.icon-only {
  padding: 0.3rem 0.45rem;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}
.icon-only:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}
.grid-striped tbody tr:nth-child(odd) {
  background: rgba(22, 27, 34, 0.55);
}
.grid-striped tbody tr:nth-child(even) {
  background: rgba(13, 17, 23, 0.65);
}
.ai-panel {
  flex: 0 0 340px;
  width: 340px;
  max-width: 42%;
  min-height: 0;
  display: flex;
  flex-direction: column;
  background: #0d1117;
  color: #e6edf3;
}
@media (max-width: 1024px) {
  .chatsql-split {
    flex-direction: column;
  }
  .ai-panel {
    flex: 0 0 auto;
    width: 100%;
    max-width: none;
    min-height: 240px;
    border-left: none;
    border-top: 1px solid #30363d;
  }
  .chatsql-main {
    border-right: none;
  }
}
.ai-panel-header {
  padding: 0.5rem 0.75rem;
  border-bottom: 1px solid #30363d;
  background: #010409;
}
.ai-panel-title {
  font-size: 0.85rem;
  font-weight: 700;
  letter-spacing: -0.02em;
}
.ai-messages {
  flex: 1;
  min-height: 0;
  padding: 0.5rem 0.6rem;
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
}
.ai-messages.scroll {
  max-height: none;
}
.ai-empty {
  margin: 0.25rem 0;
  line-height: 1.4;
}
.ai-msg {
  display: flex;
  width: 100%;
}
.ai-msg-user {
  justify-content: flex-end;
}
.ai-msg-assistant {
  justify-content: flex-start;
}
.ai-bubble {
  max-width: 100%;
  border-radius: 10px;
  font-size: 0.8rem;
  line-height: 1.45;
}
.ai-bubble-user {
  background: #21262d;
  border: 1px solid #30363d;
  padding: 0.5rem 0.65rem;
  color: #e6edf3;
}
.ai-bubble-assistant {
  background: #0b0e14;
  border: 1px solid #30363d;
  padding: 0.5rem 0.6rem 0.55rem;
  width: 100%;
}
.ai-assist-text {
  margin: 0 0 0.4rem;
  color: #c9d1d9;
  white-space: pre-wrap;
  word-break: break-word;
}
.ai-sql-block {
  margin: 0 0 0.5rem;
  padding: 0.45rem 0.5rem;
  border-radius: 6px;
  background: #161b22;
  border: 1px solid #30363d;
  color: #79c0ff;
  font-size: 0.75rem;
  line-height: 1.4;
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 12rem;
  overflow: auto;
}
.ai-assist-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.4rem 0.75rem;
}
.apply-sql-btn {
  border: none;
  border-radius: 6px;
  padding: 0.3rem 0.65rem;
  font-size: 0.78rem;
  font-weight: 600;
  cursor: pointer;
  background: #1f6feb;
  color: #fff;
}
.apply-sql-btn:hover {
  background: #388bfd;
}
.linkish-btn {
  border: none;
  background: none;
  padding: 0.25rem 0;
  font-size: 0.78rem;
  color: #58a6ff;
  cursor: pointer;
  text-decoration: none;
}
.linkish-btn:hover {
  text-decoration: underline;
}
.ai-composer {
  flex-shrink: 0;
  border-top: 1px solid #30363d;
  background: #010409;
  padding: 0.5rem 0.6rem 0.6rem;
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}
.ai-composer-input {
  width: 100%;
  min-height: 3.2rem;
  max-height: 8rem;
  resize: vertical;
  font-size: 0.8rem;
  line-height: 1.4;
  font-family: inherit;
  background: #161b22;
  color: #e6edf3;
  border: 1px solid #30363d;
  border-radius: 8px;
  padding: 0.45rem 0.5rem;
}
.ai-composer-input:focus {
  outline: none;
  border-color: #58a6ff;
  box-shadow: 0 0 0 1px #58a6ff40;
}
.ai-composer-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
}
.tiny {
  font-size: 0.7rem;
}
.ai-send {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 2.2rem;
  height: 2.2rem;
  padding: 0;
  border-radius: 8px;
}
.explain-body {
  max-height: min(50vh, 400px);
  margin: 0.25rem 0 0;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-word;
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
.left-rail .list li:hover {
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
.qtabs-saved {
  background: #0b121e;
  border-bottom: 1px solid #1e2836;
  padding-bottom: 0.5rem;
}
.error {
  color: #f85149;
  margin: 0.25rem 0;
}
.side-list {
  height: 200px;
  border-top: 1px solid #30363d;
  padding: 0.5rem;
}
.side-list.full {
  flex: 1;
  height: auto;
  min-height: 0;
  display: flex;
  flex-direction: column;
  border-top: 1px solid #30363d;
}
.list-fill {
  flex: 1;
  min-height: 0;
  max-height: none;
}
.is-saved-grid.side-list {
  background: #0b121e;
  border-top: 1px solid #1e2836;
  padding: 0;
}
.saved-queries-layer {
  padding: 1rem 1rem 1.25rem;
  min-height: 0;
  flex: 1;
  display: block;
  background: #0b121e;
}
.saved-queries-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 1rem;
  align-content: start;
}
.saved-queries-empty {
  margin: 2rem 1.5rem;
  color: #94a3b8;
  font-size: 0.875rem;
  line-height: 1.4;
  text-align: center;
  max-width: 22rem;
  margin-left: auto;
  margin-right: auto;
}
.saved-query-card {
  position: relative;
  background: #161f2e;
  border-radius: 12px;
  border: 1px solid #1e293b;
  padding: 1rem 0.9rem 0.75rem;
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
  cursor: pointer;
  transition:
    border-color 0.15s ease,
    box-shadow 0.15s ease;
  text-align: left;
}
.saved-query-card:hover {
  border-color: #334155;
  box-shadow: 0 6px 24px rgba(0, 0, 0, 0.3);
}
.saved-query-card-hd {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.4rem;
  margin-bottom: 0.6rem;
}
.saved-query-titles {
  min-width: 0;
  flex: 1;
}
.saved-query-title {
  margin: 0;
  font-size: 0.9rem;
  font-weight: 600;
  color: #ffffff;
  line-height: 1.3;
  letter-spacing: -0.01em;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.saved-query-meta {
  margin: 0.2rem 0 0;
  font-size: 0.72rem;
  color: #94a3b8;
  line-height: 1.35;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.saved-query-menu {
  position: relative;
  flex-shrink: 0;
  margin: -0.2rem 0 0 0;
}
.saved-query-dots {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 1.9rem;
  height: 1.9rem;
  padding: 0;
  border-radius: 6px;
  background: transparent;
  color: #94a3b8;
  cursor: pointer;
  border: 1px solid transparent;
}
.saved-query-dots:hover {
  color: #e2e8f0;
  background: #1e293b;
  border-color: #334155;
}
.saved-query-menu-dd {
  position: absolute;
  top: 100%;
  right: 0;
  z-index: 20;
  margin-top: 4px;
  min-width: 7rem;
  padding: 0.3rem 0;
  border-radius: 8px;
  border: 1px solid #1e293b;
  background: #0f172a;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.4);
}
.saved-query-menu-dd button {
  display: block;
  width: 100%;
  margin: 0;
  padding: 0.4rem 0.75rem;
  border: none;
  text-align: left;
  background: none;
  color: #e2e8f0;
  font-size: 0.8rem;
  cursor: pointer;
}
.saved-query-menu-dd button:hover {
  background: #1e293b;
  color: #f87171;
}
.saved-query-code {
  font-size: 0.7rem;
  line-height: 1.5;
  margin: 0 0 0.65rem;
  padding: 0.5rem 0.55rem;
  border-radius: 8px;
  background: #0b121e;
  border: 1px solid #1e293b;
  color: #cbd5e1;
  min-height: 2.4rem;
  max-height: 4.35rem;
  overflow: hidden;
  white-space: pre-wrap;
  word-break: break-word;
}
.saved-query-code :deep(.sql-hl-kw) {
  color: #38bdf8;
  font-weight: 500;
}
.saved-query-code :deep(.sql-hl-id) {
  color: #2dd4bf;
}
.saved-query-code :deep(.sql-hl-str) {
  color: #86efac;
}
.saved-query-code :deep(.sql-hl-num) {
  color: #fcd34d;
}
.saved-query-foot {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  margin-top: auto;
  padding-top: 0.1rem;
}
.saved-query-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 0.35rem;
  min-width: 0;
}
.saved-tag {
  display: inline-block;
  font-size: 0.65rem;
  font-weight: 500;
  line-height: 1.2;
  padding: 0.15rem 0.5rem;
  border-radius: 9999px;
  background: #1e293b;
  color: #94a3b8;
}
.saved-query-run {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 2.1rem;
  height: 2.1rem;
  margin: 0 0 0 auto;
  padding: 0;
  flex-shrink: 0;
  border: none;
  border-radius: 8px;
  background: rgba(56, 189, 248, 0.12);
  color: #38bdf8;
  cursor: pointer;
  transition: background 0.12s ease;
}
.saved-query-run:hover {
  background: rgba(56, 189, 248, 0.2);
  color: #7dd3fc;
}
.saved-query-run:focus-visible {
  outline: 2px solid #38bdf8;
  outline-offset: 2px;
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
.hint {
  padding: 1rem;
  color: #8b949e;
}
</style>
