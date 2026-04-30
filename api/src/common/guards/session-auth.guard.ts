import {
  Injectable,
  CanActivate,
  ExecutionContext,
  UnauthorizedException,
} from '@nestjs/common';
import { Request } from 'express';
import '@/types/express-session';

@Injectable()
export class SessionAuthGuard implements CanActivate {
  canActivate(context: ExecutionContext) {
    const req = context.switchToHttp().getRequest<Request>();
    if (!req.session?.userId) {
      throw new UnauthorizedException();
    }
    return true;
  }
}
