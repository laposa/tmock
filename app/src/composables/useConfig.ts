import type { AppConfig } from '@/plugins/config/config.interfaces';
import { inject } from 'vue';

export function useConfig(): AppConfig {
  return inject('config') as AppConfig;
}
