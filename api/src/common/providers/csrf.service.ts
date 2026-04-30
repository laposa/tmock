import { Injectable } from '@nestjs/common';
import { doubleCsrf } from 'csrf-csrf';
import { AppConfig, InjectConfig } from '@/app.config';

@Injectable()
export class CsrfService {
  public readonly doubleCsrfProtection;
  public readonly generateCsrfToken;

  constructor(@InjectConfig() config: AppConfig) {
    const { doubleCsrfProtection, generateCsrfToken } = doubleCsrf({
      getSecret: () => config.sessionSecret,
      getSessionIdentifier: (req) => req.session?.id ?? '',
      cookieName: '__csrf',
      cookieOptions: {
        httpOnly: true,
        sameSite: 'lax' as const,
        secure: config.env === 'production',
        path: '/',
      },
      getCsrfTokenFromRequest: (req) => req.headers['x-csrf-token'] as string,
      skipCsrfProtection: (req) => {
        const url = req.originalUrl || req.url;
        return url.startsWith('/api/auth/login') || url.startsWith('/api/proxy');
      },
    });
    this.doubleCsrfProtection = doubleCsrfProtection;
    this.generateCsrfToken = generateCsrfToken;
  }
}
