import 'express';

declare module 'express' {
  interface Request {
    clientToken?: { clientId: number };
  }
}
