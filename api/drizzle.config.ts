import 'dotenv/config';
import type { Config } from 'drizzle-kit';
export default {
  schema: './database/schema.ts',
  out: './database/migrations',
  dialect: 'postgresql',
  dbCredentials: {
    host: process.env.DATABASE_HOST!,
    port: +process.env.DATABASE_PORT!,
    database: process.env.DATABASE_NAME!,
    user: process.env.DATABASE_USER!,
    password: process.env.DATABASE_PASSWORD!,
    ssl: process.env.DATABASE_ENABLE_SSL === 'true',
  },
  verbose: true,
  strict: true,
  migrations: {
    table: 'mp_migrations',
    schema: 'public',
  },
} satisfies Config;
