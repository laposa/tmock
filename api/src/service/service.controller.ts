import {
  Body,
  Controller,
  Delete,
  Get,
  Param,
  Patch,
  Post,
  UseGuards,
  UsePipes,
  ValidationPipe,
} from '@nestjs/common';
import { ServiceService } from './service.service';
import { ResponseMessage } from '@/common/dtos/ResponseMessage.dto';
import { CreateServiceDto, PatchServiceDto } from './dtos';
import { ApiKeyGuard } from '@/common/guards/api-key.guard';
import { ServiceDto } from 'database/schema';

@Controller('service')
@UseGuards(ApiKeyGuard)
export class ServiceController {
  constructor(private readonly service: ServiceService) {}

  /**
   * List all services
   */
  @Get('/')
  async getAll(): Promise<ServiceDto[]> {
    return this.service.getAll();
  }

  /**
   * Create a new service
   */
  @Post('/')
  @UsePipes(ValidationPipe)
  async create(
    @Body() body: CreateServiceDto,
  ): Promise<{ message: string; service: ServiceDto }> {
    const service = await this.service.create(body);
    return { message: 'Service created', service };
  }

  /**
   * (Partially) Update a service
   */
  @Patch('/:path')
  @UsePipes(ValidationPipe)
  async update(
    @Param('path') path: string,
    @Body() body: PatchServiceDto,
  ): Promise<{ message: string; service: ServiceDto }> {
    const service = await this.service.update(path, body);
    return { message: 'Service updated', service };
  }

  /**
   * Delete a service
   */
  @Delete('/:path')
  async delete(@Param('path') path: string): Promise<ResponseMessage> {
    await this.service.delete(path);
    return { message: 'Service deleted' };
  }
}
