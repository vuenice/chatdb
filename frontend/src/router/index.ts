import { createRouter, createWebHistory } from 'vue-router'
import { routes } from 'vue-router/auto-routes'
import { useAuthStore } from '../stores/auth'

const router = createRouter({
  history: createWebHistory(),
  routes,
})

router.beforeEach((to) => {
  const auth = useAuthStore()
  if (to.meta?.requiresAuth && !auth.token) {
    if (auth.hasUsers === false) {
      return { name: 'register', query: { redirect: to.fullPath } }
    }
    return { name: 'login', query: { redirect: to.fullPath } }
  }
  if ((to.name === 'login' || to.name === 'register') && auth.token) {
    return { name: 'workbench' }
  }
  if (to.name === 'login' && !auth.token && auth.hasUsers === false) {
    return { name: 'register', query: to.query }
  }
})

export default router