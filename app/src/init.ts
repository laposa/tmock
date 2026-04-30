import './assets/main.css';
import '@mdi/font/css/materialdesignicons.css';
import 'vuetify/styles';

import { createApp } from 'vue';
import { createPinia } from 'pinia';

import App from './App.vue';
import router from './router.ts';
import VueCodemirror from 'vue-codemirror';
import { createVuetify } from 'vuetify';
import { configPlugin } from './plugins/config/config.plugin';
import type { AppConfig } from './plugins/config/config.interfaces';
import { useAuthStore } from './stores/auth.store';
import { useAuthApi } from './apis/useAuthApi';

export const initTmockApp = (config: AppConfig) => {
  const app = createApp(App);

  const pinia = createPinia();
  app.use(pinia);
  app.use(configPlugin, config);
  app.use(router);
  app.use(createVuetify());
  app.use(VueCodemirror, {
    autofocus: false,
  });

  // Auth navigation guard
  router.beforeEach(async (to, _from, next) => {
    const authStore = useAuthStore();

    // Try to restore session on first load
    if (!authStore.sessionChecked && !authStore.isAuthenticated && !to.meta.public) {
      authStore.sessionChecked = true;
      try {
        const authApi = useAuthApi();
        const data = await authApi.getMe();
        authStore.setUser(data.user);
        const csrfToken = await authApi.getCsrfToken();
        authStore.setCsrfToken(csrfToken);
      } catch {
        // Not authenticated
      }
    }

    if (to.meta.public) {
      if (authStore.isAuthenticated) {
        return next('/');
      }
      return next();
    }

    if (!authStore.isAuthenticated) {
      return next('/login');
    }

    if (to.meta.admin && !authStore.isAdmin) {
      return next('/');
    }

    next();
  });

  router.isReady().then(() => {
    app.mount('#app');
  });
};
