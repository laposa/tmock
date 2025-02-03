import { Module } from '@nestjs/common';
import { ProxyService } from './proxy.service';
import { ProxyController } from './proxy.controller';
import { TemplateService } from './services/template.service';

@Module({
  providers: [ProxyService, TemplateService],
  controllers: [ProxyController],
  exports: [ProxyService],
})
export class ProxyModule {}
