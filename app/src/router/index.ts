import { createRouter, createWebHistory } from 'vue-router'
import HomeView from '../views/HomeView.vue'
import ClientsView from '@/views/ClientsView.vue'
import ScenariosView from '@/views/ScenariosView.vue'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/',
      name: 'home',
      component: HomeView,
    },
    {
      path: '/clients',
      name: 'clients',
      component: ClientsView,
    },
    {
      path: '/scenarios',
      name: 'scenarios',
      component: ScenariosView,
    },
  ],
})

export default router
