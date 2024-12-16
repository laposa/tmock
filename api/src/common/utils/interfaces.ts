import { IncomingMessage, ServerResponse } from 'http';

export type ProxyResponse = ServerResponse<IncomingMessage> & {
  startTime?: number;
};
