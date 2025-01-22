import { Injectable } from '@nestjs/common';
import { AppDatabase, InjectDb } from '../providers/database.provider';
import { clients, clientsScenarios, ClientWithScenariosDto } from 'database/schema';
import { and, eq, ne, inArray } from 'drizzle-orm';
import { CreateClientDto, PatchClientDto } from '@/client/dtos';

@Injectable()
export class ClientsRepository {
  constructor(@InjectDb() private db: AppDatabase) {}

  async getAll() {
    return this.db.select().from(clients);
  }

  async disableAll() {
    await this.db.update(clients).set({ enabled: false });
  }

  async getEnabled() {
    return this.db.query.clients.findMany({
        where: eq(clients.enabled, true),
    });
  }

  async getByName(name: string) {
    return this.db.query.clients.findFirst({
      where: eq(clients.name, name),
    });
  }

  async getById(id: number) {
    return this.db.query.clients.findFirst({
      where: eq(clients.id, id),
    });
  }

  async create(data: CreateClientDto) {
    const client = await this.db.insert(clients).values(data).returning({
      id: clients.id,
    });

    return client;
  }

  async update(id: number, data: PatchClientDto) {
    const client = await this.db
      .update(clients)
      .set(data)
      .where(eq(clients.id, id))
      .returning({
        id: clients.id,
      });
    
    if(data.scenarios) {
      await this.updateScenarios(client[0].id, data.scenarios);
    }
    
    return client;
  }

  async updateScenarios(id: number, scenarios: number[]) {
    await this.db.delete(clientsScenarios).where(eq(clientsScenarios.clientId, id));

    if (!scenarios.length) {
      return
    }
    
    return this.db
      .insert(clientsScenarios)
      .values(scenarios.map((scenarioId) => ({ clientId: id, scenarioId })));
  }

  async delete(id: number) {
    return this.db.delete(clients).where(eq(clients.id, id));
  }
}
