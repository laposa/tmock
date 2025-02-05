import {
  ArgumentsHost,
  Catch,
  ExceptionFilter,
  HttpException,
  HttpStatus,
} from '@nestjs/common';
import { omit } from 'lodash';
import { Response } from 'express';
import { AppLoggerService } from '../utils/app-logger.service';

@Catch()
export class AppFilter implements ExceptionFilter {
  private readonly logger = new AppLoggerService(AppFilter.name);

  catch(exception: Error & Record<string, any>, host: ArgumentsHost): void {
    console.log(exception);
    const ctx = host.switchToHttp();
    const res = ctx.getResponse<Response>();

    const status =
      exception instanceof HttpException
        ? exception.getStatus()
        : HttpStatus.INTERNAL_SERVER_ERROR;

    let errorMessage = exception.message;
    let errorDetails: object = {};

    // Http based errors (nestjs and custom exceptions)
    if (exception instanceof HttpException && exception.getResponse()) {
      const errResponse = exception.getResponse() as Record<string, any>;

      // custom error details
      const details = omit(errResponse, [
        'message',
        'statusCode',
        'error',
      ]) as object;

      if (Object.keys(details).length > 0) {
        errorDetails = details;
      }

      // validation error
      if (
        Array.isArray(errResponse.message) &&
        errResponse.message.length > 0
      ) {
        errorDetails = errResponse.message;
      }
    }

    if (Object.keys(errorDetails).length > 0) {
      errorMessage += `, Details: ${JSON.stringify(errorDetails)}`;
      this.logger.error(errorMessage);
    } else {
      this.logger.error(errorMessage, exception.stack);
    }

    res.status(status).json({
      status,
      message: exception.message,
      details: Object.keys(errorDetails).length > 0 ? errorDetails : undefined,
    });
  }
}
