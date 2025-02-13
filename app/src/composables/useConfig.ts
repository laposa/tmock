import type { AppConfig } from '@/plugins/config/config.interfaces';

export function useConfig(): AppConfig {
  return inject('config') as AppConfig;
}
