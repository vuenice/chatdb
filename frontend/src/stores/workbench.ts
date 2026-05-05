import { defineStore } from 'pinia'
import { ref } from 'vue'

type Nav = 'tables' | 'queries' | 'users' | 'history' | 'operations'
type QueriesTab = 'chatsql' | 'saved' | 'running'
type TableTab = 'structure' | 'data' | 'indexes'

export const useWorkbenchStore = defineStore('workbench', () => {
  const nav = ref<Nav>('tables')
  const queriesTab = ref<QueriesTab>('chatsql')
  const tableTab = ref<TableTab>('structure')

  function setNav(value: Nav) {
    nav.value = value
  }

  function setQueriesTab(value: QueriesTab) {
    queriesTab.value = value
  }

  function setTableTab(value: TableTab) {
    tableTab.value = value
  }

  return { nav, queriesTab, tableTab, setNav, setQueriesTab, setTableTab }
})