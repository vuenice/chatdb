<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter, useRoute, RouterLink } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const router = useRouter()
const route = useRoute()
const auth = useAuthStore()

const labels = ref<string[]>([])
const connectionName = ref('')
const username = ref('')
const password = ref('')
const error = ref('')
const loading = ref(false)
const pageLoading = ref(true)

onMounted(async () => {
  error.value = ''
  try {
    labels.value = await auth.loadConnectionLabels()
    if (labels.value.length === 0) {
      const r = route.query.redirect
      await router.replace(
        typeof r === 'string' && r
          ? { path: '/register', query: { redirect: r } }
          : { path: '/register' },
      )
      return
    }
    connectionName.value = labels.value[0] ?? ''
  } catch (e: unknown) {
    const err = e as { response?: { data?: { error?: string } } }
    error.value = err.response?.data?.error || 'Failed to load connection labels'
  } finally {
    pageLoading.value = false
  }
})

async function submit() {
  error.value = ''
  loading.value = true
  try {
    if (!connectionName.value.trim()) {
      error.value = 'Select a connection label'
      return
    }
    await auth.login(connectionName.value, username.value, password.value)
    await auth.loadPublicHealth()
    const redir = route.query.redirect
    await router.push(typeof redir === 'string' && redir ? redir : '/')
  } catch (e: unknown) {
    const err = e as { response?: { data?: { error?: string } } }
    error.value = err.response?.data?.error || 'Request failed'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="auth-page">
    <p v-if="pageLoading" class="muted">Loading…</p>
    <form v-else class="card" @submit.prevent="submit">
      <h1>ChatDB</h1>
      <p class="muted">PostgreSQL &amp; MySQL viewer</p>
      <label>
        Connection label
        <select v-model="connectionName" required>
          <option v-for="l in labels" :key="l" :value="l">{{ l }}</option>
        </select>
      </label>
      <label>
        Username
        <input v-model="username" type="text" required autocomplete="username" />
      </label>
      <label>
        Password
        <input v-model="password" type="password" autocomplete="current-password" />
      </label>

      <p v-if="error" class="error">{{ error }}</p>
      <button type="submit" class="primary" :disabled="loading || pageLoading">{{ loading ? '…' : 'Login' }}</button>
      <p v-if="auth.hasUsers !== false" class="footer">
        <RouterLink to="/register">Register new connection</RouterLink>
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
h1 {
  margin: 0;
  font-size: 1.5rem;
}
.muted {
  margin: 0;
  color: #8b949e;
  font-size: 0.9rem;
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
