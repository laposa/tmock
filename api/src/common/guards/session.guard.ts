import { Request } from 'express';
import { Injectable, CanActivate, ExecutionContext } from '@nestjs/common';
import '@/types/express-session';

@Injectable()
export class SessionGuard implements CanActivate {
  canActivate(context: ExecutionContext) {
    const req = context.switchToHttp().getRequest<Request>();
    return !!req.session?.userId;
  }
}
