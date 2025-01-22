import { Request } from 'express';
import { Injectable, CanActivate, ExecutionContext } from '@nestjs/common';
import { AppConfig, InjectConfig } from '@/app.config';

@Injectable()
export class ApiKeyGuard implements CanActivate {
  constructor(@InjectConfig() protected readonly config: AppConfig) {}

  async canActivate(context: ExecutionContext) {
    if (!this.config.apiKey) {
      return true;
    }

    const req = context.switchToHttp().getRequest<Request>();
    const apiKeyHeader = req.headers.apikey;

    if (!apiKeyHeader) {
      return false;
    }

    if (apiKeyHeader === this.config.apiKey) {
      return true;
    }

    return false;
  }
}
