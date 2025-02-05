import { Injectable } from '@nestjs/common';
import { AppDatabase, InjectDb } from '../providers/database.provider';
import {
  clients,
  clientsScenarios,
  ClientWithScenariosDto,
  ClientWithScenarioIdsDto,
} from 'database/schema';
import { eq, and, ne, asc } from 'drizzle-orm';
import { CreateClientDto, PatchClientDto } from '@/client/dtos';

@Injectable()
export class ClientsRepository {
  constructor(@InjectDb() private db: AppDatabase) {}

  async getAll() {
    return (
      await this.db.query.clients.findMany({
        with: {
          scenarios: {
            with: {
              scenario: {
                columns: { id: true },
              },
            },
          },
        },
        orderBy: [asc(clients.id)],
      })
    ).map((c) => {
      return {
        ...c,
        scenarios: c.scenarios.map((s) => s.scenario.id),
      } as ClientWithScenarioIdsDto;
    });
  }

  async disableAll() {
    await this.db.update(clients).set({ enabled: false });
  }

  async getEnabled() {
    return (
      await this.db.query.clients.findMany({
        where: and(eq(clients.enabled, true), ne(clients.condition, {})),
        with: {
          scenarios: {
            with: {
              scenario: {},
            },
          },
        },
        orderBy: [asc(clients.id)],
      })
    ).map((c) => {
      return {
        ...c,
        scenarios: c.scenarios.map((s) => s.scenario),
      } as ClientWithScenariosDto;
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

    if (data.scenarios) {
      await this.updateScenarios(client[0].id, data.scenarios);
    }

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

    if (data.scenarios) {
      await this.updateScenarios(client[0].id, data.scenarios);
    }

    return client;
  }

  async updateScenarios(id: number, scenarios: number[]) {
    await this.db
      .delete(clientsScenarios)
      .where(eq(clientsScenarios.clientId, id));

    if (!scenarios.length) {
      return;
    }

    return this.db
      .insert(clientsScenarios)
      .values(scenarios.map((scenarioId) => ({ clientId: id, scenarioId })));
  }

  async delete(id: number) {
    return this.db.delete(clients).where(eq(clients.id, id));
  }
}
