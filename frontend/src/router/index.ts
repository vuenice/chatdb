import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import { routes as autoRoutes } from 'vue-router/auto-routes'
import { useAuthStore } from '../stores/auth'

// unplugin-vue-router derives route names/paths from filenames, which gives
// us things like `/LoginView` and `/workbench/`. The rest of the app (and the
// auth guard below) refers to `login`, `register`, `workbench`, so remap the
// generated routes to those names and clean paths.
const aliasByGeneratedName: Record<
  string,
  { name: string; path: string; meta?: Record<string, unknown> }
> = {
  '/LoginView': { name: 'login', path: '/login' },
  '/RegisterView': { name: 'register', path: '/register' },
  '/workbench/': { name: 'workbench', path: '/workbench', meta: { requiresAuth: true } },
}

const routes: RouteRecordRaw[] = (autoRoutes as RouteRecordRaw[]).map((r) => {
  const a = aliasByGeneratedName[String(r.name)]
  if (!a) return r
  return { ...r, ...a, meta: { ...(r.meta ?? {}), ...(a.meta ?? {}) } }
})

routes.push({ path: '/', redirect: '/workbench' })
routes.push({ path: '/:pathMatch(.*)*', redirect: '/workbench' })

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
