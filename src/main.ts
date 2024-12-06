import * as express from 'express';
import { NestFactory } from '@nestjs/core';
import { SwaggerModule, DocumentBuilder } from '@nestjs/swagger';
import { NestExpressApplication } from '@nestjs/platform-express';
import helmet from 'helmet';
import { AppModule } from './app.module';
import appConfig from './app.config';
import { AppLoggerService } from './common/utils/app-logger.service';
import { AppFilter } from './common/filters/app.filter';

async function bootstrap() {
  const app = await NestFactory.create<NestExpressApplication>(AppModule, {
    bodyParser: false,
  });
  const config = app.get(appConfig.KEY);

  app.enableCors();
  app.use(express.json({ limit: '10mb' }));
  app.use(express.urlencoded({ extended: true, limit: '10mb' }));

  app.use(helmet());

  app.set('trust proxy', true);
  app.disable('x-powered-by');

  app.useGlobalFilters(new AppFilter());
  const logger = app.get(AppLoggerService);

  const openApiConf = new DocumentBuilder()
    .setTitle('Testing Mock Proxy')
    .build();
  const document = SwaggerModule.createDocument(app, openApiConf);
  SwaggerModule.setup('swagger', app, document);

  await app.listen(config.port, () => {
    logger.log(`Started listening on ${config.port}`, 'NestApplication');
  });
}
bootstrap();
