<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { useRouter, RouterLink } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const router = useRouter()
const auth = useAuthStore()

const error = ref('')
const loading = ref(false)

const errorHint = computed(() =>
  error.value.includes('email and password required')
    ? 'Please fill Host, Database, and Database username. Database password is optional.'
    : error.value.includes('Access denied for user')
    ? 'hint: Incorrect Database username or Database password'
    : '',
)

const conn = ref({
  connection_name: '',
  driver: 'postgres' as 'postgres' | 'mysql',
  host: '127.0.0.1',
  port: 5432,
  database: '',
  ssl_mode: 'disable',
  db_username: '',
  db_password: '',
})

watch(
  () => conn.value.driver,
  (d) => {
    conn.value.port = d === 'mysql' ? 3306 : 5432
  },
)

async function submit() {
  error.value = ''
  loading.value = true
  try {
    if (!conn.value.host.trim() || !conn.value.database.trim() || !conn.value.db_username.trim()) {
      error.value = 'Host, Database, and Database username are required'
      return
    }
    await auth.register({
      connection_name: conn.value.connection_name || undefined,
      driver: conn.value.driver,
      host: conn.value.host,
      port: conn.value.port,
      database: conn.value.database,
      ssl_mode: conn.value.ssl_mode,
      read_username: conn.value.db_username,
      read_password: conn.value.db_password,
    })
    conn.value.db_password = ''
    await auth.loadPublicHealth()
    const redir = router.currentRoute.value.query.redirect
    await router.push(typeof redir === 'string' && redir ? redir : '/')
  } catch (e: unknown) {
    const err = e as { response?: { data?: { error?: string } } }
    const msg = err.response?.data?.error || 'Request failed'
    error.value = msg
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="auth-page">
    <form class="card wide" @submit.prevent="submit">
      <h1>ChatDB</h1>
      <p class="muted">Connect your database</p>
      <label
        >Connection name (optional)
        <input v-model="conn.connection_name" type="text" placeholder="e.g. production" />
      </label>
      <label
        >Driver
        <select v-model="conn.driver">
          <option value="postgres">PostgreSQL</option>
          <option value="mysql">MySQL / MariaDB</option>
        </select>
      </label>
      <label>Host <input v-model="conn.host" required /></label>
      <label>Port <input v-model.number="conn.port" type="number" /></label>
      <label
        >Database Name
        <input v-model="conn.database" required placeholder="Default database" />
      </label>
      <label v-if="conn.driver === 'postgres'"
        >SSL mode <input v-model="conn.ssl_mode" placeholder="disable"
      /></label>
      <label
        >Database username
        <input v-model="conn.db_username" required autocomplete="off" placeholder="e.g. root" />
      </label>
      <label
        >Database password
        <input
          v-model="conn.db_password"
          type="password"
          autocomplete="new-password"
          placeholder="leave empty for no password"
        />
      </label>

      <p v-if="error" class="error">{{ error }}</p>
      <p v-if="errorHint" class="error-hint">{{ errorHint }}</p>
      <button type="submit" class="primary" :disabled="loading">{{ loading ? '…' : 'Continue' }}</button>
      <p v-if="auth.hasUsers !== false" class="footer">
        <RouterLink to="/login">Already have an account? Sign in</RouterLink>
      </p>
    </form>
  </div>
</template>

<style scoped>
.auth-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #0f1419;
  color: #e6edf3;
  padding: 1rem;
}
.card {
  width: 100%;
  max-width: 400px;
  padding: 2rem;
  border-radius: 12px;
  background: #161b22;
  border: 1px solid #30363d;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}
.card.wide {
  max-width: 480px;
}
h1 {
  margin: 0;
  font-size: 1.5rem;
}
.muted {
  margin: 0;
  color: #8b949e;
  font-size: 0.9rem;
}
.section-title {
  margin: 0.5rem 0 0;
  font-size: 0.85rem;
  font-weight: 600;
  color: #e6edf3;
}
label {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  font-size: 0.85rem;
  color: #8b949e;
}
input,
select {
  padding: 0.5rem 0.6rem;
  border-radius: 6px;
  border: 1px solid #30363d;
  background: #0d1117;
  color: #e6edf3;
}
.primary {
  margin-top: 0.5rem;
  padding: 0.6rem;
  border: none;
  border-radius: 6px;
  background: #238636;
  color: #fff;
  font-weight: 600;
  cursor: pointer;
}
.primary:disabled {
  opacity: 0.6;
  cursor: default;
}
.error {
  color: #f85149;
  margin: 0;
  font-size: 0.85rem;
}
.error-hint {
  color: #d29922;
  margin: -0.25rem 0 0;
  font-size: 0.85rem;
}
.footer {
  margin: 0;
  font-size: 0.85rem;
  text-align: center;
}
.footer a {
  color: #58a6ff;
  text-decoration: none;
}
.footer a:hover {
  text-decoration: underline;
}
</style>
