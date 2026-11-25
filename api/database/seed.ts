import * as bcrypt from 'bcrypt';
import 'dotenv/config';
import { count, eq } from 'drizzle-orm';
import { drizzle } from 'drizzle-orm/node-postgres';
import { Pool } from 'pg';
import * as schema from './schema';

async function seed() {
  const email = process.env.ADMIN_EMAIL || 'hugo@laposa.ie';
  const password = process.env.ADMIN_PASSWORD;

  if (!password) {
    console.error('ADMIN_PASSWORD environment variable is required.');
    console.error(
      'Usage: ADMIN_EMAIL=admin@example.com ADMIN_PASSWORD=changeme npm run database:seed',
    );
    process.exit(1);
  }

  const pool = new Pool({
    host: process.env.DATABASE_HOST,
    port: +process.env.DATABASE_PORT!,
    user: process.env.DATABASE_USER,
    password: process.env.DATABASE_PASSWORD,
    database: process.env.DATABASE_NAME,
    ssl:
      process.env.DATABASE_ENABLE_SSL === 'true'
        ? { rejectUnauthorized: false }
        : false,
  });

  const db = drizzle(pool, { schema });

  const [{ count: userCount }] = await db
    .select({ count: count() })
    .from(schema.users)
    .where(eq(schema.users.deleted, false));

  if (userCount > 0) {
    console.log(`Database already has ${userCount} user(s). Skipping seed.`);
    await pool.end();
    return;
  }

  const hashedPassword = await bcrypt.hash(password, 10);
  await db.insert(schema.users).values({
    name: 'Admin',
    email,
    password: hashedPassword,
    admin: true,
  });

  console.log(`Created admin user: ${email}`);
  await pool.end();
}

seed().catch((err) => {
  console.error('Seed failed:', err);
  process.exit(1);
});
