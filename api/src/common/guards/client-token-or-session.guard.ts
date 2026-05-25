import { Injectable, CanActivate, ExecutionContext } from '@nestjs/common';
import { Reflector } from '@nestjs/core';
import { SessionGuard } from './session.guard';
import { ClientTokenGuard } from './client-token.guard';
import { ALLOWS_CLIENT_TOKEN } from '@/common/decorators/allows-client-token.decorator';

@Injectable()
export class ClientTokenOrSessionGuard implements CanActivate {
  constructor(
    private readonly reflector: Reflector,
    private readonly clientToken: ClientTokenGuard,
    private readonly session: SessionGuard,
  ) {}

  async canActivate(ctx: ExecutionContext) {
    const allowsClientToken =
      this.reflector.get<boolean>(ALLOWS_CLIENT_TOKEN, ctx.getHandler()) ===
      true;

    if (allowsClientToken && (await this.clientToken.canActivate(ctx))) {
      return true;
    }

    return this.session.canActivate(ctx);
  }
}
