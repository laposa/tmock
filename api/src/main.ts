import * as fs from 'fs';
import * as path from 'path';
import * as express from 'express';
import * as session from 'express-session';
import * as cookieParser from 'cookie-parser';
import { NestFactory } from '@nestjs/core';
import { SwaggerModule, DocumentBuilder } from '@nestjs/swagger';
import { NestExpressApplication } from '@nestjs/platform-express';
import helmet from 'helmet';
import { AppModule } from './app.module';
import appConfig, { AppConfig } from './app.config';
import { AppLoggerService } from './common/utils/app-logger.service';
import { AppFilter } from './common/filters/app.filter';
import { frontendMiddleware } from './common/middlewares/frontend.middleware';
import { CsrfService } from './common/providers/csrf.service';

async function bootstrap() {
  const app = await NestFactory.create<NestExpressApplication>(AppModule, {
    bodyParser: false,
  });
  const config = app.get<AppConfig>(appConfig.KEY);

  app.setGlobalPrefix('api');
  app.use(frontendMiddleware);
  app.enableCors({
    origin: true,
    credentials: true,
  });

  app.use(cookieParser());
  app.use(
    session({
      secret: config.sessionSecret,
      resave: false,
      saveUninitialized: false,
      cookie: {
        httpOnly: true,
        sameSite: 'lax',
        secure: config.env === 'production',
        maxAge: 24 * 60 * 60 * 1000, // 24 hours
      },
    }),
  );

  app.use(express.json({ limit: '10mb' }));
  app.use(express.urlencoded({ extended: true, limit: '10mb' }));

  const csrfService = app.get(CsrfService);
  app.use(csrfService.doubleCsrfProtection);

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
  const newContent = indexFileContent.replace(/\$\{API_ENDPOINT\}/g, '/api');

  fs.writeFileSync(indexFilePath, newContent);
}

bootstrap().catch((err) => {
  console.error(err);
  process.exit(1);
});
