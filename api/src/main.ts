import * as fs from 'fs';
import * as path from 'path';
import * as express from 'express';
import { NestFactory } from '@nestjs/core';
import { SwaggerModule, DocumentBuilder } from '@nestjs/swagger';
import { NestExpressApplication } from '@nestjs/platform-express';
import helmet from 'helmet';
import { AppModule } from './app.module';
import appConfig, { AppConfig } from './app.config';
import { AppLoggerService } from './common/utils/app-logger.service';
import { AppFilter } from './common/filters/app.filter';
import { frontendMiddleware } from './common/middlewares/frontend.middleware';

async function bootstrap() {
  const app = await NestFactory.create<NestExpressApplication>(AppModule, {
    bodyParser: false,
  });
  const config = app.get<AppConfig>(appConfig.KEY);

  app.setGlobalPrefix('api');
  app.use(frontendMiddleware);
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

  injectEnvs(config);

  await app.listen(config.port, () => {
    logger.log(`Started listening on ${config.port}`, 'NestApplication');
  });
}

// inject environment variables for the frontend
function injectEnvs(config: AppConfig) {
  const publicPath = path.resolve('public/assets');

  if (!fs.existsSync(publicPath)) {
    return;
  }

  const files = fs.readdirSync(publicPath);
  const indexFile = files.find(
    (file) => file.includes('index-') && file.endsWith('.js'),
  );

  if (!indexFile) {
    return;
  }

  const indexFilePath = path.resolve(publicPath, indexFile);
  const indexFileContent = fs.readFileSync(indexFilePath, 'utf8');
  let newContent = indexFileContent.replace(/\$\{API_ENDPOINT\}/g, '/api');
  newContent = newContent.replace(/\$\{API_KEY\}/g, config.apiKey);

  fs.writeFileSync(indexFilePath, newContent);
}

bootstrap().catch((err) => {
  console.error(err);
  process.exit(1);
});
