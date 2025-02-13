import { createRouter, createWebHistory } from 'vue-router';
import ClientsView from '@/views/ClientsView.vue';
import ScenariosView from '@/views/ScenariosView.vue';

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      name: 'clients',
      path: '/',
      component: ClientsView,
    },
    {
      name: 'scenarios',
      path: '/scenarios',
      component: ScenariosView,
    },
  ],
});

export default router;
