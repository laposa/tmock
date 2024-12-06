import { Injectable } from '@nestjs/common';
import { AppDatabase, InjectDb } from '../providers/database.provider';
import { ServiceDto, ServiceWithScenariosDto } from 'database/schema';

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

  async getByPath(path: string): Promise<ServiceDto> {
    return this.db.query.services.findFirst({
      where: (service, { eq }) => eq(service.path, path),
    });
  }
}