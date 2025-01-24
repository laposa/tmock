import { ClientRequest, IncomingMessage, ServerResponse } from 'http';
import { Inject, Injectable } from '@nestjs/common';
import { CACHE_MANAGER, Cache } from '@nestjs/cache-manager';
import {
  createProxyMiddleware,
  fixRequestBody,
  responseInterceptor,
} from 'http-proxy-middleware';
import { ServicesRepository } from '@/common/repositories/services.repository';
import { AppConfig, InjectConfig } from '@/app.config';
import { evalClientConditions, evalRequestCondition, getClientIp } from '@/common/utils/helpers';
import { ScenarioDto, ServiceWithScenariosDto } from 'database/schema';
import { AppLoggerService } from '@/common/utils/app-logger.service';
import { TemplateService } from './services/template.service';
import { ProxyResponse } from '@/common/utils/interfaces';
import { ClientsRepository } from '@/common/repositories/clients.repository';

@Injectable()
export class ProxyService {
  private readonly logger = new AppLoggerService(ProxyService.name);
  public readonly middleware: ReturnType<typeof createProxyMiddleware>;

  constructor(
    private servicesRepository: ServicesRepository,
    private clientsRepository: ClientsRepository,
    private templateService: TemplateService,
    @InjectConfig() private config: AppConfig,
    @Inject(CACHE_MANAGER) private cacheManager: Cache,
  ) {
    this.middleware = this.getProxyMiddleware();
  }

  private getProxyMiddleware() {
    return createProxyMiddleware({
      router: async (req) => this.proxyRouter(req),
      selfHandleResponse: true,
      changeOrigin: true,
      on: {
        proxyReq: (proxyReq, req, res) => this.onProxyReq(proxyReq, req, res),
        proxyRes: responseInterceptor(
          async (responseBuffer, proxyRes, req, res) =>
            this.onProxyRes(responseBuffer, proxyRes, req, res),
        ),
      },
    });
  }

  private async proxyRouter(req: IncomingMessage) {
    const service = await this.getServiceByReq(req);
    if (!service) {
      return 'http://service-not-found';
    }

    return service.upstreamUrl;
  }

  private async onProxyReq(
    proxyReq: ClientRequest,
    req: IncomingMessage,
    res: ProxyResponse,
  ) {
    res.startTime = Date.now();

    // prepare the request for the upstream service - needs to happen before any async code
    // remove /proxy/:service-name from the upstream path
    proxyReq.path = proxyReq.path.replace(/\/proxy\/[^/]+/, '');
    if (this.config.servicesApiKey) {
      proxyReq.setHeader('Apikey', this.config.servicesApiKey);
    }

    // remove forwarded headers
    if (this.config.proxy.removeForwardedHeaders) {
      proxyReq.removeHeader('x-forwarded-host');
      proxyReq.removeHeader('x-forwarded-port');
      proxyReq.removeHeader('x-forwarded-proto');
      proxyReq.removeHeader('x-forwarded-scheme');
      proxyReq.removeHeader('x-scheme');
      proxyReq.removeHeader('x-forwarded-server');
    }

    fixRequestBody(proxyReq, req);

    if (proxyReq.host === 'service-not-found') {
      this.writeResponse(proxyReq, res, 404, 'Service not found');
      this.logProxyRequest(req, res);
      return;
    }
  }

  private async onProxyRes(
    responseBuffer: Buffer,
    proxyRes: IncomingMessage,
    req: IncomingMessage,
    res: ServerResponse<IncomingMessage>,
  ) {
    const scenario = await this.getScenario(req);
    let response: Buffer | string = responseBuffer;

    if (scenario) {
      if (scenario.responseHeaders) {
        this.applyHeaders(
          res,
          scenario.responseHeaders as Record<string, string>,
        );
      }

      if (scenario.responseCode) {
        res.statusCode = scenario.responseCode;
      }

      if (scenario && scenario.responseBody) {
        response = await this.templateService.parse(scenario.responseBody);
        this.removeEtagHeaders(res);
      }
    }

    this.logProxyRequest(req, res, scenario);
    return response;
  }

  private async getScenario(req: IncomingMessage) {
    const service = await this.getServiceByReq(req);
    if (!service) {
      return null;
    }

    let clients = await this.clientsRepository.getEnabled();

    clients = clients.filter((c) => evalClientConditions(c.condition, req));
    if (!clients || clients.length === 0) {
      return null;
    }

    const clientScenariosIds = clients.flatMap((c) => c.scenarios.map((s) => s.id));

    const scenarioPath = req.url!.split('/').slice(3).join('/');
    return service.scenarios.find((s) => {
      if (!clientScenariosIds.includes(s.id)) {
        return false;
      }

      if (
        s.requestPath &&
        s.requestPath.toLowerCase() !== scenarioPath.toLowerCase()
      ) {
        return false;
      }

      if (
        s.requestMethod &&
        s.requestMethod.toLowerCase() !== req.method!.toLowerCase()
      ) {
        return false;
      }

      if (s.requestCondition) {
        return evalRequestCondition({ request: req }, s.requestCondition);
      }

      return true;
    });
  }

  private writeResponse(
    proxyReq: ClientRequest,
    res: ServerResponse,
    statusCode: number,
    body: string,
    headers?: Record<string, string>,
  ) {
    proxyReq.destroy();

    if (headers) {
      this.applyHeaders(res, headers);
    }

    res.statusCode = statusCode;
    res.write(body);
    res.end();
  }

  private async logProxyRequest(
    req: IncomingMessage,
    res: ProxyResponse,
    scenario?: ScenarioDto | null,
  ) {
    const ip = getClientIp(req);
    const url = req.url;

    let message = `${ip} ${req.method} ${url} ${res.statusCode}`;
    if (scenario) {
      const mocked: string[] = [];
      if (scenario.responseCode) mocked.push('status');
      if (scenario.responseHeaders) mocked.push('headers');
      if (scenario.responseBody) mocked.push('body');

      message += ` [scenario ${scenario.name}; mocked: ${mocked.join(', ')}]`;
    }

    const ms = Date.now() - (res?.startTime || 0);
    message += ` ${ms}ms`;

    this.logger.log(message);
  }

  private applyHeaders(res: ServerResponse, headers: Record<string, string>) {
    Object.entries(headers).forEach(([key, value]) => {
      if (value === null && res.hasHeader(key)) {
        res.removeHeader(key);
        return;
      }

      res.setHeader(key, value);
    });
  }

  private removeEtagHeaders(res: ServerResponse) {
    res.removeHeader('etag');
    res.removeHeader('if-match');
    res.removeHeader('if-none-match');
  }

  private async getServiceByReq(req: IncomingMessage) {
    const path = req.url!.split('/')[2];
    const services = await this.getServices();
    return services.find((s) => s.path === path);
  }

  async getServices() {
    const cached =
      await this.cacheManager.get<ServiceWithScenariosDto[]>('services');

    if (cached && !this.config.disableCache) {
      return cached;
    }

    const services = await this.servicesRepository.getScenarios();
    // services upstream overrides
    services.forEach((service) => {
      const override = this.config.upstreamOverrides.find(
        (o) => o.path === service.path,
      );
      if (override) {
        service.upstreamUrl = override.url;
      }
    });

    await this.cacheManager.set('services', services, 1000 * 60 * 10);
    return services;
  }

  async clearServicesCache() {
    await this.cacheManager.del('services');
  }

  async clearClientsCache() {
    await this.cacheManager.del('clients');
  }
}
