import { Injectable } from '@nestjs/common';
import { AppDatabase, InjectDb } from '../providers/database.provider';
import { scenarios } from 'database/schema';
import { and, eq } from 'drizzle-orm';
import { PatchScenarioDto } from '@/scenario/dtos/PatchScenario.dto';
import { CreateScenarioDto } from '@/scenario/dtos/CreateScenario.dto';

@Injectable()
export class ScenariosRepository {
  constructor(@InjectDb() private db: AppDatabase) {}

  async getAll() {
    return this.db.select().from(scenarios);
  }

  async getById(id: number) {
    return this.db.query.scenarios.findFirst({
      where: eq(scenarios.id, id),
    });
  }

  async getByNameAndService(name: string, service: string) {
    return this.db.query.scenarios.findFirst({
      where: and(eq(scenarios.name, name), eq(scenarios.service, service)),
    });
  }

  async create(data: CreateScenarioDto) {
    return this.db.insert(scenarios).values(data).returning({
      id: scenarios.id,
    });
  }

  async update(id: number, data: PatchScenarioDto) {
    return this.db
      .update(scenarios)
      .set(data)
      .where(eq(scenarios.id, id))
      .returning({
        id: scenarios.id,
      });
  }

  async delete(id: number) {
    return this.db.delete(scenarios).where(eq(scenarios.id, id));
  }
}
