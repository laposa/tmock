import type { App } from 'vue';
import type { AppConfig } from './config.interfaces';

export const configPlugin = {
  install: (app: App, config: AppConfig) => {
    const conf: AppConfig = {
      ...config,
      baseUrl: window.location.origin,

      version: {
        major: 1,
        minor: 1,
        build: config.buildTimestamp.split('T')[0].replace(/-/g, ''),
        timestamp: config.buildTimestamp,
      },
    };

    app.provide('config', conf);
    app.config.globalProperties.$config = conf;
  },
};
