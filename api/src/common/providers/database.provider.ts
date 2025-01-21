import { Pool } from 'pg';
import { Inject } from '@nestjs/common';
import { drizzle } from 'drizzle-orm/node-postgres';
import appConfig, { AppConfig } from '@/app.config';
import * as schema from 'database/schema';

export const DB_PROVIDER_KEY = 'DRIZZLE_DB_PROVIDER';

export const databaseProvider = {
  provide: DB_PROVIDER_KEY,
  useFactory: (config: AppConfig) => {
    const { host, ssl, user, password, port, databaseName } = config.database;
    
    const pool = new Pool({
      host,
      port,
      user,
      password,
      database: databaseName,
      ssl: ssl ? { rejectUnauthorized: false } : false,
    });
    return drizzle(pool, { schema, logger: true });
  },
  inject: [appConfig.KEY],
};

export type AppDatabase = ReturnType<typeof databaseProvider.useFactory>;

export const InjectDb = () => Inject(DB_PROVIDER_KEY);
