import AccountsView from '@/views/AccountsView.vue';
import ClientsView from '@/views/ClientsView.vue';
import LoginView from '@/views/LoginView.vue';
import MyAccountView from '@/views/MyAccountView.vue';
import ScenariosView from '@/views/ScenariosView.vue';
import ServicesView from '@/views/ServicesView.vue';
import { createRouter, createWebHistory } from 'vue-router';

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      name: 'login',
      path: '/login',
      component: LoginView,
      meta: { public: true },
    },
    {
      name: 'services',
      path: '/services',
      component: ServicesView,
    },
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
    {
      name: 'my-account',
      path: '/my-account',
      component: MyAccountView,
    },
    {
      name: 'accounts',
      path: '/accounts',
      component: AccountsView,
      meta: { admin: true },
    },
  ],
});

export default router;
