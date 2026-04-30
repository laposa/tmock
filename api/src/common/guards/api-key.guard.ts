import { Request } from 'express';
import { Injectable, CanActivate, ExecutionContext } from '@nestjs/common';
import { AppConfig, InjectConfig } from '@/app.config';
import '@/types/express-session';

@Injectable()
export class ApiKeyGuard implements CanActivate {
  constructor(@InjectConfig() protected readonly config: AppConfig) {}

  canActivate(context: ExecutionContext) {
    const req = context.switchToHttp().getRequest<Request>();

    if (req.session?.userId) {
      return true;
    }

    if (!this.config.apiKey) {
      return true;
    }

    const apiKeyHeader = req.headers.apikey;
    return apiKeyHeader === this.config.apiKey;
  }
}
