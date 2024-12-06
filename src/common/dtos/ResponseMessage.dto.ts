export class ResponseMessage {
  message: string;
  status?: 'error' | 'warning' | 'info' | 'success';
  httpCode?: number;
}
