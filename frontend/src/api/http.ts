import axios from 'axios'

const baseURL = import.meta.env.VITE_API_BASE || ''

export const http = axios.create({
  baseURL,
  headers: { 'Content-Type': 'application/json' },
})

http.interceptors.request.use((config) => {
  const raw = localStorage.getItem('chatdb_token')
  if (raw) {
    const token = raw.startsWith('Bearer ') ? raw : `Bearer ${raw}`
    config.headers.Authorization = token
  }
  return config
})

http.interceptors.response.use(
  (r) => r,
  (err) => {
    if (err.response?.status === 401) {
      localStorage.removeItem('chatdb_token')
      localStorage.removeItem('chatdb_user')
      const p = window.location.pathname
      if (!p.includes('/login') && !p.includes('/register')) {
        window.location.href = '/login'
      }
    }
    return Promise.reject(err)
  },
)
