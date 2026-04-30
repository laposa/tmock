import { Injectable } from '@nestjs/common';
import { AppDatabase, InjectDb } from '../providers/database.provider';
import { users } from 'database/schema';
import { eq, and, asc, count } from 'drizzle-orm';

@Injectable()
export class UsersRepository {
  constructor(@InjectDb() private db: AppDatabase) {}

  async getAll() {
    return this.db.query.users.findMany({
      where: eq(users.deleted, false),
      orderBy: [asc(users.id)],
    });
  }

  async getById(id: number) {
    return this.db.query.users.findFirst({
      where: and(eq(users.id, id), eq(users.deleted, false)),
    });
  }

  async getByEmail(email: string) {
    return this.db.query.users.findFirst({
      where: and(eq(users.email, email), eq(users.deleted, false)),
    });
  }

  async create(data: { name: string; email: string; password: string; admin: boolean }) {
    const result = await this.db.insert(users).values(data).returning();
    return result[0];
  }

  async update(id: number, data: Partial<{ name: string; email: string; password: string; admin: boolean }>) {
    await this.db.update(users).set(data).where(eq(users.id, id));
    return this.getById(id);
  }

  async softDelete(id: number) {
    await this.db.update(users).set({ deleted: true }).where(eq(users.id, id));
  }

  async count() {
    const result = await this.db
      .select({ count: count() })
      .from(users)
      .where(eq(users.deleted, false));
    return result[0].count;
  }

  async countAdmins() {
    const result = await this.db
      .select({ count: count() })
      .from(users)
      .where(and(eq(users.deleted, false), eq(users.admin, true)));
    return result[0].count;
  }
}
