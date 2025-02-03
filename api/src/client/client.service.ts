import { BadRequestException, Injectable, NotFoundException } from '@nestjs/common';
import { ClientsRepository } from '@/common/repositories/clients.repository';
import { CreateClientDto, PatchClientDto } from './dtos';
import { isEmptyCondition } from '@/common/utils/helpers';

@Injectable()
export class ClientService {
  constructor(
    private clientsRepository: ClientsRepository,
  ) {}

  async create(data: CreateClientDto) {
    const client = await this.clientsRepository.getByName(
      data.name,
    );

    if (client) {
      throw new NotFoundException(
        `Client [${data.name}] already exists.`,
      );
    }

    const { id } = (await this.clientsRepository.create(data))[0];
    return (await this.clientsRepository.getById(id))!;
  }

  async update(id: number, data: PatchClientDto) {
    const client = await this.clientsRepository.getById(id);
    if (!client) {
      throw new NotFoundException(`Client with id [${id}] was not found.`);
    }

    if (data.enabled === true && isEmptyCondition(client.condition || {})) {
      throw new BadRequestException(
        'Cannot enable client without a condition.',
      );
    }

    await this.clientsRepository.update(id, data);
    return (await this.clientsRepository.getById(id))!;
  }

  async getById(id: number) {
    const client = await this.clientsRepository.getById(id);
    if (!client) {
      throw new NotFoundException(`Client with id [${id}] was not found.`);
    }

    return client;
  }

  async upsert(data: CreateClientDto) {
    const client = await this.clientsRepository.getByName(
      data.name,
    );

    if (client) {
      await this.update(client.id, data);
      return {
        client: (await this.clientsRepository.getById(client.id))!,
        updated: true,
      };
    }

    const { id } = (await this.clientsRepository.create(data))[0];
    return {
      client: (await this.clientsRepository.getById(id))!,
      updated: false,
    };
  }

  async delete(id: number) {
    const client = await this.clientsRepository.getById(id);
    if (!client) {
      throw new NotFoundException(`Client with id [${id}] was not found.`);
    }

    return this.clientsRepository.delete(id);
  }

  async disableAll() {
    await this.clientsRepository.disableAll();
  }

  async getEnabled() {
    return this.clientsRepository.getEnabled();
  }

  async getAll() {
    return this.clientsRepository.getAll();
  }
}

