import { All, Controller, Next, Req, Res } from '@nestjs/common';
import { ProxyService } from './proxy.service';
import { NextFunction, Request, Response } from 'express';
import { RequestScenario } from './proxy.interfaces';

@Controller('proxy')
export class ProxyController {
  constructor(private proxyService: ProxyService) {}

  /**
   * Proxy all requests based on service path
   */
  @All('*path')
  async proxy(
    @Req() req: Request,
    @Res() res: Response<any, { mock: RequestScenario }>,
    @Next() next: NextFunction,
  ) {
    // load scenario and client for the request before hitting the proxy
    // because onProxyReq can't handle async code
    res.locals.mock = await this.proxyService.getScenario(req);

    return this.proxyService.middleware(req, res, next);
  }
}
