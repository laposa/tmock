import { Global, Module } from '@nestjs/common';
import { APP_INTERCEPTOR } from '@nestjs/core';
import { AppLoggerService } from './utils/app-logger.service';
import { LoggingInterceptor } from './interceptors/logging.interceptor';
import { databaseProvider } from './providers/database.provider';
import { ScenariosRepository } from './repositories/scenarios.repository';
import { ServicesRepository } from './repositories/services.repository';
import { OptionsRepository } from './repositories/options.repository';
import { ClientsRepository } from './repositories/clients.repository';

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
  ],
  exports: [
    AppLoggerService,
    databaseProvider,
    ScenariosRepository,
    ServicesRepository,
    OptionsRepository,
    ClientsRepository,
  ],
})
export class CommonModule {}
