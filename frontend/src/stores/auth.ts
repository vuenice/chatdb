import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { http } from '../api/http'

export type Role = 'viewer' | 'analyst' | 'engineer'

export interface User {
  id: number
  name: string
  email: string
  role: Role
}

export const useAuthStore = defineStore('auth', () => {
  const token = ref<string | null>(localStorage.getItem('chatdb_token'))
  const user = ref<User | null>(null)
  /** null until /api/health is loaded */
  const hasUsers = ref<boolean | null>(null)

  const stored = localStorage.getItem('chatdb_user')
  if (stored) {
    try {
      user.value = JSON.parse(stored) as User
    } catch {
      /* ignore */
    }
  }

  const isEngineer = computed(() => user.value?.role === 'engineer')

  function persist() {
    if (token.value) localStorage.setItem('chatdb_token', token.value)
    else localStorage.removeItem('chatdb_token')
    if (user.value) localStorage.setItem('chatdb_user', JSON.stringify(user.value))
    else localStorage.removeItem('chatdb_user')
  }

  async function login(email: string, password: string) {
    const { data } = await http.post('/api/login', { email, password })
    token.value = data.token as string
    user.value = data.user as User
    persist()
  }

  async function register(payload: {
    name: string
    email: string
    password: string
    role: Role
    connection_name?: string
    driver?: string
    host: string
    port?: number
    database: string
    ssl_mode?: string
    read_username: string
    read_password: string
  }) {
    const { data } = await http.post('/api/register', payload)
    token.value = data.token as string
    user.value = data.user as User
    persist()
  }

  function logout() {
    token.value = null
    user.value = null
    persist()
  }

  async function loadPublicHealth() {
    const { data } = await http.get<{ ok?: boolean; has_users?: boolean }>('/api/health')
    if (typeof data.has_users === 'boolean') {
      hasUsers.value = data.has_users
    } else {
      hasUsers.value = null
    }
  }

  return { token, user, hasUsers, isEngineer, login, register, logout, persist, loadPublicHealth }
})
