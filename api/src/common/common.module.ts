import { Global, Module } from '@nestjs/common';
import { APP_INTERCEPTOR } from '@nestjs/core';
import { AppLoggerService } from './utils/app-logger.service';
import { LoggingInterceptor } from './interceptors/logging.interceptor';
import { databaseProvider } from './providers/database.provider';
import { ScenariosRepository } from './repositories/scenarios.repository';
import { ServicesRepository } from './repositories/services.repository';
import { OptionsRepository } from './repositories/options.repository';
import { ClientsRepository } from './repositories/clients.repository';
import { UsersRepository } from './repositories/users.repository';
import { CsrfService } from './providers/csrf.service';
import { SessionGuard } from './guards/session.guard';
import { ClientTokenGuard } from './guards/client-token.guard';
import { ClientTokenOrSessionGuard } from './guards/client-token-or-session.guard';
import { ClientTokenScopeGuard } from './guards/client-token-scope.guard';

@Global()
@Module({
  providers: [
    AppLoggerService,
    {
      provide: APP_INTERCEPTOR,
      useClass: LoggingInterceptor,
    },
    databaseProvider,
    ScenariosRepository,
    ServicesRepository,
    OptionsRepository,
    ClientsRepository,
    UsersRepository,
    CsrfService,
    SessionGuard,
    ClientTokenGuard,
    ClientTokenOrSessionGuard,
    ClientTokenScopeGuard,
  ],
  exports: [
    AppLoggerService,
    databaseProvider,
    ScenariosRepository,
    ServicesRepository,
    OptionsRepository,
    ClientsRepository,
    UsersRepository,
    CsrfService,
    SessionGuard,
    ClientTokenGuard,
    ClientTokenOrSessionGuard,
    ClientTokenScopeGuard,
  ],
})
export class CommonModule {}
