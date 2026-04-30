import {
  BadRequestException,
  Injectable,
  NotFoundException,
} from '@nestjs/common';
import { ServicesRepository } from '@/common/repositories/services.repository';
import { CreateServiceDto, PatchServiceDto } from './dtos';

@Injectable()
export class ServiceService {
  constructor(private servicesRepository: ServicesRepository) {}

  async getAll() {
    return this.servicesRepository.getAll();
  }

  async create(data: CreateServiceDto) {
    const existing = await this.servicesRepository.getByPath(data.path);

    if (existing) {
      throw new BadRequestException(
        `Service with path [${data.path}] already exists.`,
      );
    }

    const [service] = await this.servicesRepository.create(data);
    return service;
  }

  async update(path: string, data: PatchServiceDto) {
    const service = await this.servicesRepository.getByPath(path);
    if (!service) {
      throw new NotFoundException(
        `Service with path [${path}] was not found.`,
      );
    }

    const [updated] = await this.servicesRepository.update(path, data);
    return updated;
  }

  async delete(path: string) {
    const service = await this.servicesRepository.getByPath(path);
    if (!service) {
      throw new NotFoundException(
        `Service with path [${path}] was not found.`,
      );
    }

    return this.servicesRepository.delete(path);
  }
}
