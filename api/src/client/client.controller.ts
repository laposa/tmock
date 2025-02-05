import {
  Body,
  Controller,
  Delete,
  Get,
  Param,
  Patch,
  Post,
  Put,
  UseGuards,
  UsePipes,
  ValidationPipe,
} from '@nestjs/common';
import { ClientService } from './client.service';

import { ResponseMessage } from '@/common/dtos/ResponseMessage.dto';
import { ProxyService } from '@/proxy/proxy.service';
import {
  ClientDto,
  ClientUpsertResponse,
  CreateClientDto,
  PatchClientDto,
  ClientResponse,
} from './dtos';
import { ApiKeyGuard } from '@/common/guards/api-key.guard';

@Controller('api/client')
@UseGuards(ApiKeyGuard)
export class ClientController {
  constructor(
    private readonly service: ClientService,
    private readonly proxyService: ProxyService,
  ) {}

  /**
   * List all available clients
   */
  @Get('/')
  async getAvailable(): Promise<ClientDto[]> {
    return this.service.getAll();
  }

  /**
   * Deactivate all clients
   */
  @Post('/deactivate/all')
  async setAllInactive(): Promise<ResponseMessage> {
    await this.service.disableAll();
    await this.proxyService.clearServicesCache();
    return { message: 'All clients deactivated' };
  }

  /**
   * List all active clients
   */
  @Get('/active')
  async getActive(): Promise<ClientDto[]> {
    return this.service.getEnabled();
  }

  /**
   * Clear Clients cache
   */
  @Post('/clear-cache')
  async clearCache(): Promise<ResponseMessage> {
    await this.proxyService.clearClientsCache();
    return { message: 'Cache cleared' };
  }

  /**
   * Create a new client
   */
  @Post('/')
  @UsePipes(ValidationPipe)
  async create(@Body() body: CreateClientDto): Promise<ClientUpsertResponse> {
    const client = await this.service.create(body);
    await this.proxyService.clearClientsCache();
    return { message: 'Client created', client };
  }

  /**
   * Update or create a client (based on name)
   */
  @Put('/')
  @UsePipes(ValidationPipe)
  async upsert(@Body() body: CreateClientDto): Promise<ClientUpsertResponse> {
    const { client, updated } = await this.service.upsert(body);
    await this.proxyService.clearClientsCache();
    return { message: `Client ${updated ? 'updated' : 'created'}`, client };
  }

  /**
   * (Partially) Update a client
   */
  @Patch('/:id')
  @UsePipes(ValidationPipe)
  async update(
    @Param('id') id: string,
    @Body() body: PatchClientDto,
  ): Promise<ClientUpsertResponse> {
    const client = await this.service.update(+id, body);
    await this.proxyService.clearClientsCache();
    return { message: 'Client updated', client };
  }

  /**
   * Get a client by ID
   */
  @Get('/:id')
  async getById(@Param('id') id: string): Promise<ClientResponse> {
    const client = await this.service.getById(+id);
    return { client };
  }

  /**
   * Delete a client by ID
   */
  @Delete('/:id')
  async delete(@Param('id') id: string): Promise<ResponseMessage> {
    await this.service.delete(+id);
    await this.proxyService.clearClientsCache();
    return { message: 'Client deleted' };
  }
}
