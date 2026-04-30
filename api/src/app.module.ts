import { Module, NestModule } from '@nestjs/common';
import { ConfigModule } from '@nestjs/config';
import { CacheModule } from '@nestjs/cache-manager';
import { AppController } from './app.controller';
import appConfig from './app.config';
import { CommonModule } from './common/common.module';
import { ScenarioModule } from './scenario/scenario.module';
import { ProxyModule } from './proxy/proxy.module';
import { ClientModule } from './client/client.module';
import { ServiceModule } from './service/service.module';

@Module({
  imports: [
    // third-party modules
    ConfigModule.forRoot({ isGlobal: true, load: [appConfig] }),
    CacheModule.register({ isGlobal: true }),

    // app modules
    CommonModule,
    ScenarioModule,
    ProxyModule,
    ClientModule,
    ServiceModule,
  ],
  controllers: [AppController],
})
export class AppModule implements NestModule {
  configure() {}
}
