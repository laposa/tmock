import {
  Injectable,
  CanActivate,
  ExecutionContext,
  UnauthorizedException,
} from '@nestjs/common';
import { Request } from 'express';
import { ClientsRepository } from '@/common/repositories/clients.repository';
import '@/types/express-request';

@Injectable()
export class ClientTokenGuard implements CanActivate {
  constructor(private readonly clientsRepository: ClientsRepository) {}

  async canActivate(context: ExecutionContext) {
    const req = context.switchToHttp().getRequest<Request>();
    const raw = req.headers['x-client-token'];
    if (typeof raw !== 'string' || !raw) return false;

    const client = await this.clientsRepository.getByToken(raw);
    if (!client) throw new UnauthorizedException();

    req.clientToken = { clientId: client.id };
    return true;
  }
}
