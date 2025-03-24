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
import {
  evalClientConditions,
  evalRequestCondition,
  getClientIp,
} from '@/common/utils/helpers';
import {
  ClientWithScenariosDto,
  ScenarioDto,
  ServiceWithScenariosDto,
} from 'database/schema';
import { AppLoggerService } from '@/common/utils/app-logger.service';
import { TemplateService } from './services/template.service';
import { ClientsRepository } from '@/common/repositories/clients.repository';
import type { RequestScenario, ProxyResponse } from './proxy.interfaces';

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
        // eslint-disable-next-line @typescript-eslint/no-misused-promises
        proxyRes: responseInterceptor(
          // eslint-disable-next-line @typescript-eslint/require-await
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

  onProxyReq(
    proxyReq: ClientRequest,
    req: IncomingMessage,
    res: ProxyResponse,
  ) {
    res.startTime = Date.now();

    // prepare the request for the upstream service - needs to happen before any async code
    // remove /api/proxy/:service-name from the upstream path
    proxyReq.path = proxyReq.path.replace(/\/api\/proxy\/[^/]+/, '');
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

    this.addTmockHeaders(proxyReq, res.locals!.mock);

    fixRequestBody(proxyReq, req);

    const { scenario } = res.locals!.mock;
    if (scenario && scenario.skipProxy && scenario.responseBody) {
      this.applyScenario(req, res);
      this.logProxyRequest(req, res, scenario.responseBody);
      this.writeResponse(
        proxyReq,
        res,
        scenario.responseCode || 200,
        scenario.responseBody,
      );
      return;
    }

    if (proxyReq.host === 'service-not-found') {
      this.writeResponse(proxyReq, res, 404, 'Service not found');
      this.logProxyRequest(req, res);
      return;
    }
  }

  private onProxyRes(
    responseBuffer: Buffer,
    proxyRes: IncomingMessage,
    req: IncomingMessage,
    res: ProxyResponse,
  ) {
    const { scenario, client } = res.locals!.mock;

    this.applyScenario(req, res);
    const resBody = scenario?.responseBody || responseBuffer;

    this.addTmockHeaders(res, { scenario, client });
    this.logProxyRequest(req, res, resBody);
    return resBody;
  }

  private applyScenario(req: IncomingMessage, res: ProxyResponse) {
    const { scenario } = res.locals!.mock;
    if (!scenario) {
      return;
    }

    if (scenario.responseHeaders) {
      this.applyHeaders(res, scenario.responseHeaders);
    }

    if (scenario.responseCode) {
      res.statusCode = scenario.responseCode;
    }

    if (scenario.responseBody) {
      this.removeEtagHeaders(res);
    }

    if (scenario.skipProxy && scenario.responseBody) {
      return;
    }

    // add defaults when skipping the proxy
    if (!scenario.responseCode) {
      scenario.responseCode = 200;
    }

    // get the content type from the request headers
    if (!res.getHeader('content-type')) {
      const contentType = req.headers['accept'] || req.headers['content-type'];

      if (contentType) {
        res.setHeader('content-type', contentType);
      }
    }
  }

  async getScenario(req: IncomingMessage): Promise<RequestScenario> {
    const service = await this.getServiceByReq(req);
    if (!service) {
      return { scenario: undefined, client: undefined };
    }

    let clients = await this.getClients();

    clients = clients.filter((c) => evalClientConditions(c.condition, req));
    if (!clients || clients.length === 0) {
      return { scenario: undefined, client: undefined };
    }

    let matchedClient = clients[0];

    const scenarioPath = req.url!.split('/').slice(4).join('/');
    const scenario = service.scenarios.find((s) => {
      const client = clients.find((c) =>
        c.scenarios.some((sc) => sc.id === s.id),
      );
      if (!client) {
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

      matchedClient = client;
      return true;
    });

    // parse the template, so it can be used in the onProxyReq
    if (scenario?.responseBody) {
      scenario.responseBody = await this.templateService.parse(
        scenario.responseBody,
      );
    }

    return { scenario, client: matchedClient };
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

  private logProxyRequest(
    req: IncomingMessage,
    res: ProxyResponse,
    responseBuffer?: Buffer | string,
  ) {
    const ip = getClientIp(req);
    const url = req.url;
    const { scenario, client } = res.locals!.mock;

    let message = `${ip} ${req.method} ${url} ${res.statusCode}`;
    if (scenario) {
      const mocked: string[] = [];
      if (scenario.responseCode) mocked.push('status');
      if (scenario.responseHeaders) mocked.push('headers');
      if (scenario.responseBody) mocked.push('body');

      let skip = '';
      if (scenario.responseBody && scenario.skipProxy) {
        skip = '; proxy skipped';
      }

      message += ` [client ${client!.name}; scenario ${scenario.name}; mocked: ${mocked.join(', ')}${skip}]`;
    }

    const ms = Date.now() - (res?.startTime || 0);
    message += ` ${ms}ms`;

    // add response size
    if (responseBuffer) {
      message += ` ${this.getHumanSize(responseBuffer.length)}`;
    }

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

  private addTmockHeaders(
    reqOrRes: ClientRequest | ServerResponse,
    mock: RequestScenario,
  ) {
    reqOrRes.setHeader('x-tmock-status', mock.scenario ? 'mocked' : 'proxied');

    if (mock.client) {
      reqOrRes.setHeader('x-tmock-client', mock.client.name);
    }

    if (mock.scenario) {
      reqOrRes.setHeader('x-tmock-scenario', mock.scenario.name);
    }
  }

  private async getServiceByReq(req: IncomingMessage) {
    const path = req.url!.split('/')[3];
    const services = await this.getServices();
    return services.find((s) => s.path === path);
  }

  private getHumanSize(bytes: number) {
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    if (bytes === 0) {
      return '0B';
    }

    const i = Math.floor(Math.log(bytes) / Math.log(1024));
    return `${(bytes / Math.pow(1024, i)).toFixed(2)}${sizes[i]}`.replace(
      '.00',
      '',
    );
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

  async getClients() {
    const cached =
      await this.cacheManager.get<ClientWithScenariosDto[]>('clients');

    if (cached && !this.config.disableCache) {
      return cached;
    }

    const clients = await this.clientsRepository.getEnabled();
    await this.cacheManager.set('clients', clients, 1000 * 60 * 10);
    return clients;
  }

  async clearServicesCache() {
    await this.cacheManager.del('services');
  }

  async clearClientsCache() {
    await this.cacheManager.del('clients');
  }
}
