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

export const initTmockApp = (config: AppConfig) => {
  const app = createApp(App);

  app.use(createPinia());
  app.use(router);
  app.use(configPlugin, config);
  app.use(createVuetify());
  app.use(VueCodemirror, {
    autofocus: false,
  })

  router.isReady().then(() => {
    app.mount('#app');
  });
};
