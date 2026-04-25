<script setup lang="ts">
import { ref } from 'vue'
import { useRouter, useRoute, RouterLink } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const router = useRouter()
const route = useRoute()
const auth = useAuthStore()

const username = ref('')
const password = ref('')
const error = ref('')
const loading = ref(false)

async function submit() {
  error.value = ''
  loading.value = true
  try {
    await auth.login(username.value, password.value)
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
    <form class="card" @submit.prevent="submit">
      <h1>ChatDB</h1>
      <p class="muted">PostgreSQL &amp; MySQL viewer</p>
      <label>
        Username
        <input v-model="username" type="text" required autocomplete="username" />
      </label>
      <label>
        Password
        <input v-model="password" type="password" required autocomplete="current-password" />
      </label>

      <p v-if="error" class="error">{{ error }}</p>
      <button type="submit" class="primary" :disabled="loading">{{ loading ? '…' : 'Login' }}</button>
      <p v-if="auth.hasUsers !== false" class="footer">
        <RouterLink to="/register">Create an account</RouterLink>
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
input {
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
