import { CacheModule } from '@nestjs/cache-manager';
import { Module, NestModule } from '@nestjs/common';
import { ConfigModule } from '@nestjs/config';
import appConfig from './app.config';
import { AppController } from './app.controller';
import { AuthModule } from './auth/auth.module';
import { ClientModule } from './client/client.module';
import { CommonModule } from './common/common.module';
import { ProxyModule } from './proxy/proxy.module';
import { ScenarioModule } from './scenario/scenario.module';
import { ServiceModule } from './service/service.module';
import { UserModule } from './user/user.module';

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
    AuthModule,
    UserModule,
    ServiceModule,
  ],
  controllers: [AppController],
})
export class AppModule implements NestModule {
  configure() {}
}
