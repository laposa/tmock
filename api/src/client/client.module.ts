import { Module } from '@nestjs/common';
import { ClientController } from './client.controller';
import { ClientService } from './client.service';
import { ProxyModule } from '@/proxy/proxy.module';

@Module({
  imports: [ProxyModule],
  controllers: [ClientController],
  providers: [ClientService],
})
export class ClientModule {}
