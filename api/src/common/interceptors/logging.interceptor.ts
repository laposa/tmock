import {
  CallHandler,
  ExecutionContext,
  Injectable,
  NestInterceptor,
} from '@nestjs/common';
import { Observable } from 'rxjs';
import { tap } from 'rxjs/operators';
import { AppLoggerService } from '../utils/app-logger.service';
import { getClientIp } from '../utils/helpers';
import { Request, Response } from 'express';

@Injectable()
export class LoggingInterceptor implements NestInterceptor {
  // log status once each ten minutes
  statusLogDelay = 10 * 60 * 1000;
  nextStatusLog = 0;

  constructor(private readonly logger: AppLoggerService) {}

  intercept(context: ExecutionContext, next: CallHandler): Observable<any> {
    const now = Date.now();

    if (context.getHandler().name === 'getStatus') {
      if (this.nextStatusLog > now) return next.handle();
      this.nextStatusLog = now + this.statusLogDelay;
    }

    if (context.getClass().name === 'ProxyController') {
      return next.handle();
    }

    return next.handle().pipe(
      tap(
        () => {
          setTimeout(() => {
            this.logRequest(now, context);
          });
        },
        () => {
          setTimeout(() => {
            this.logRequest(now, context);
          });
        },
      ),
    );
  }

  private logRequest(executionStart: number, context: ExecutionContext) {
    const req = context.switchToHttp().getRequest<Request>();
    const res = context.switchToHttp().getResponse<Response>();
    const contextName = `${context.getClass().name} - ${context.getHandler().name}`;

    const ip = getClientIp(req);
    const url = req.originalUrl || req.url;

    this.logger.log(
      `${ip} ${req.method} ${url} ${res.statusCode} ${Date.now() - executionStart}ms`,
      contextName,
    );
  }
}
