import { All, Controller, Next, Req, Res } from '@nestjs/common';
import { ProxyService } from './proxy.service';

@Controller('api/proxy')
export class ProxyController {
  constructor(private proxyService: ProxyService) {}

  /**
   * Proxy all requests based on service path
   */
  @All('*path')
  proxy(@Req() req, @Res() res, @Next() next) {
    return this.proxyService.middleware(req, res, next);
  }
}
