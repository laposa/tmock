import { Inject } from '@nestjs/common';
import { ConfigType, registerAs } from '@nestjs/config';

const appConfig = registerAs('app', () => {
  const upstreamOverrides = process.env.SERVICES_UPSTREAM_OVERRIDES
    ? process.env.SERVICES_UPSTREAM_OVERRIDES.split(',').map((override) => {
        const [path, url] = override.split('::');
        return { path, url };
      })
    : [];

  return {
    env: process.env.NODE_ENV!,
    port: +process.env.PORT! || 3000,
    apiKey: process.env.API_KEY!,
    disableCache: process.env.DISABLE_CACHE === 'true',
    upstreamOverrides,
    servicesApiKey: process.env.SERVICES_API_KEY!,
    database: {
      host: process.env.DATABASE_HOST!,
      port: +process.env.DATABASE_PORT!,
      databaseName: process.env.DATABASE_NAME!,
      user: process.env.DATABASE_USER!,
      password: process.env.DATABASE_PASSWORD!,
      ssl: process.env.DATABASE_ENABLE_SSL === 'true' ? 'require' : false,
    },
    proxy: {
      removeForwardedHeaders: process.env.PROXY_REMOVE_FORWARDED_HEADERS,
    },
  };
});

export const InjectConfig = () => Inject(appConfig.KEY);

export type AppConfig = ConfigType<typeof appConfig>;

export default appConfig;
