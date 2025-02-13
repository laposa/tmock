import type { App } from 'vue';
import type { AppConfig } from './config.interfaces';

export const configPlugin = {
  install: (app: App, config: AppConfig) => {
    app.provide('config', config);
    app.config.globalProperties.$config = config;
  },
};
