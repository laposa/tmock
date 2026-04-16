import { Injectable } from '@nestjs/common';
import { AppDatabase, InjectDb } from '../providers/database.provider';
import { services, ServiceWithScenariosDto } from 'database/schema';
import { eq, asc } from 'drizzle-orm';

@Injectable()
export class ServicesRepository {
  constructor(@InjectDb() private db: AppDatabase) {}

  async getScenarios(): Promise<ServiceWithScenariosDto[]> {
    return this.db.query.services.findMany({
      with: {
        scenarios: {},
      },
    });
  }

  async getAll() {
    return this.db.query.services.findMany({
      orderBy: [asc(services.name)],
    });
  }

  async getByPath(path: string) {
    return this.db.query.services.findFirst({
      where: (service, { eq }) => eq(service.path, path),
    });
  }

  async create(data: { name: string; upstreamUrl: string; path: string }) {
    return this.db.insert(services).values(data).returning();
  }

  async update(
    path: string,
    data: Partial<{ name: string; upstreamUrl: string; path: string }>,
  ) {
    return this.db
      .update(services)
      .set(data)
      .where(eq(services.path, path))
      .returning();
  }

  async delete(path: string) {
    return this.db.delete(services).where(eq(services.path, path));
  }
}
