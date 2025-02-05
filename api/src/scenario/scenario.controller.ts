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
import { ScenarioService } from './scenario.service';
import {
  ScenarioResponse,
  ScenarioUpsertResponse,
  ScenariosListResponse,
  CreateScenarioDto,
  PatchScenarioDto,
} from './dtos';
import { ResponseMessage } from '@/common/dtos/ResponseMessage.dto';
import { ProxyService } from '@/proxy/proxy.service';
import { ApiKeyGuard } from '@/common/guards/api-key.guard';

@Controller('scenario')
@UseGuards(ApiKeyGuard)
export class ScenarioController {
  constructor(
    private readonly service: ScenarioService,
    private readonly proxyService: ProxyService,
  ) {}

  /**
   * List all available scenarios
   */
  @Get('/')
  async getAvailable(): Promise<ScenariosListResponse> {
    return this.service.getAll();
  }

  /**
   * Clear Services cache
   */
  @Post('/clear-cache')
  async clearCache(): Promise<ResponseMessage> {
    await this.proxyService.clearServicesCache();
    return { message: 'Cache cleared' };
  }

  /**
   * Create a new scenario
   */
  @Post('/')
  @UsePipes(ValidationPipe)
  async create(
    @Body() body: CreateScenarioDto,
  ): Promise<ScenarioUpsertResponse> {
    const scenario = await this.service.create(body);
    await this.proxyService.clearServicesCache();
    return { message: 'Scenario created', scenario };
  }

  /**
   * Update or create a scenario (based on name and service)
   */
  @Put('/')
  @UsePipes(ValidationPipe)
  async upsert(
    @Body() body: CreateScenarioDto,
  ): Promise<ScenarioUpsertResponse> {
    const { scenario, updated } = await this.service.upsert(body);
    await this.proxyService.clearServicesCache();
    return { message: `Scenario ${updated ? 'updated' : 'created'}`, scenario };
  }

  /**
   * (Partially) Update a scenario
   */
  @Patch('/:id')
  @UsePipes(ValidationPipe)
  async update(
    @Param('id') id: string,
    @Body() body: PatchScenarioDto,
  ): Promise<ScenarioUpsertResponse> {
    const scenario = await this.service.update(+id, body);
    await this.proxyService.clearServicesCache();
    return { message: 'Scenario updated', scenario };
  }

  /**
   * Get a scenario by ID
   */
  @Get('/:id')
  async getById(@Param('id') id: string): Promise<ScenarioResponse> {
    const scenario = await this.service.getById(+id);
    return { scenario };
  }

  /**
   * Delete a scenario by ID
   */
  @Delete('/:id')
  async delete(@Param('id') id: string): Promise<ResponseMessage> {
    await this.service.delete(+id);
    await this.proxyService.clearServicesCache();
    return { message: 'Scenario deleted' };
  }
}
