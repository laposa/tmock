import { Injectable, NotFoundException } from '@nestjs/common';
import { ScenariosRepository } from '@/common/repositories/scenarios.repository';
import {
  ScenariosListResponse,
  CreateScenarioDto,
  PatchScenarioDto,
} from './dtos';
import { ScenarioDto } from 'database/schema';
import { ServicesRepository } from '@/common/repositories/services.repository';

@Injectable()
export class ScenarioService {
  constructor(
    private scenariosRepository: ScenariosRepository,
    private servicesRepository: ServicesRepository,
  ) {}

  async create(data: CreateScenarioDto) {
    const service = await this.servicesRepository.getByPath(data.service);
    if (!service) {
      throw new NotFoundException(`Service [${data.service}] was not found.`);
    }

    const scenario = await this.scenariosRepository.getByNameAndService(
      data.name,
      data.service,
    );

    if (scenario) {
      throw new NotFoundException(
        `Scenario [${data.name}] within the [${data.service}] already exists.`,
      );
    }

    const { id } = (await this.scenariosRepository.create(data))[0];
    return (await this.scenariosRepository.getById(id))!;
  }

  async update(id: number, data: PatchScenarioDto) {
    const scenario = await this.scenariosRepository.getById(id);
    if (!scenario) {
      throw new NotFoundException(`Scenario with id [${id}] was not found.`);
    }

    await this.scenariosRepository.update(id, data);
    return (await this.scenariosRepository.getById(id))!;
  }

  async getById(id: number) {
    const scenario = await this.scenariosRepository.getById(id);
    if (!scenario) {
      throw new NotFoundException(`Scenario with id [${id}] was not found.`);
    }

    return scenario;
  }

  async upsert(data: CreateScenarioDto) {
    const service = await this.servicesRepository.getByPath(data.service);
    if (!service) {
      throw new NotFoundException(`Service [${data.service}] was not found.`);
    }

    const scenario = await this.scenariosRepository.getByNameAndService(
      data.name,
      data.service,
    );

    if (scenario) {
      await this.update(scenario.id, data);
      return {
        scenario: (await this.scenariosRepository.getById(scenario.id))!,
        updated: true,
      };
    }

    const { id } = (await this.scenariosRepository.create(data))[0];
    return {
      scenario: (await this.scenariosRepository.getById(id))!,
      updated: false,
    };
  }

  async delete(id: number) {
    const scenario = await this.scenariosRepository.getById(id);
    if (!scenario) {
      throw new NotFoundException(`Scenario with id [${id}] was not found.`);
    }

    return this.scenariosRepository.delete(id);
  }

  async getAll() {
    return this.mapScenarios(await this.scenariosRepository.getAll());
  }

  mapScenarios(scenarios: ScenarioDto[]): ScenariosListResponse {
    const services: ScenariosListResponse['services'] = [];

    scenarios.forEach((scenario) => {
      scenario.responseBody =
        scenario.responseBody && scenario.responseBody.length > 255
          ? scenario.responseBody.slice(0, 255) + '...'
          : scenario.responseBody;

      const service = services.find((s) => s.path === scenario.service);
      if (service) {
        service.scenarios.push(scenario);
      } else {
        services.push({
          path: scenario.service,
          scenarios: [scenario],
        });
      }
    });

    return { services };
  }
}
