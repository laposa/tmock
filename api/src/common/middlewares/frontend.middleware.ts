import { NextFunction, Response } from 'express';
import * as path from 'path';

const allowedExt = [
  '.js',
  '.ico',
  '.css',
  '.png',
  '.jpg',
  '.woff2',
  '.woff',
  '.eot',
  '.ttf',
  '.svg',
];

const resolvePath = (file: string) => path.resolve(`public/${file}`);

export function frontendMiddleware(
  req: Request,
  res: Response,
  next: NextFunction,
) {
  const { url } = req;
  if (url.startsWith('/api')) {
    next();
  } else if (allowedExt.filter((ext) => url.indexOf(ext) > 0).length > 0) {
    // remove query from the url
    const urlParts = url.split('?');
    res.sendFile(resolvePath(urlParts[0]));
  } else {
    res.sendFile(resolvePath('index.html'));
  }
}
