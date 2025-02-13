import { Controller, Get } from '@nestjs/common';

@Controller()
export class AppController {
  // Status endpoint for probes
  @Get('/status')
  getStatus(): { msg: 'ok' } {
    return { msg: 'ok' };
  }
}
