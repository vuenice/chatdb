import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import { useAuthStore } from './stores/auth'
import './style.css'

const pinia = createPinia()
const app = createApp(App)
app.use(pinia)

const auth = useAuthStore()
try {
  await auth.loadPublicHealth()
} catch {
  /* offline or server down; router will send user to login if needed */
}

app.use(router)
app.mount('#app')
