import {
  Injectable,
  CanActivate,
  ExecutionContext,
  ForbiddenException,
} from '@nestjs/common';
import { Request } from 'express';
import '@/types/express-request';

const ALLOWED_FIELDS = new Set(['enabled', 'scenarios']);

@Injectable()
export class ClientTokenScopeGuard implements CanActivate {
  canActivate(ctx: ExecutionContext) {
    const req = ctx.switchToHttp().getRequest<Request>();
    if (!req.clientToken) return true;

    const targetId = Number(req.params.id);
    if (targetId !== req.clientToken.clientId) {
      throw new ForbiddenException('Token not authorized for this client');
    }

    const body = (req.body ?? {}) as Record<string, unknown>;
    const extras = Object.keys(body).filter((k) => !ALLOWED_FIELDS.has(k));
    if (extras.length) {
      throw new ForbiddenException(
        `Token cannot modify: ${extras.join(', ')}`,
      );
    }

    return true;
  }
}
