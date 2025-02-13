import { ClientDto } from '.';

export interface ClientUpsertResponse {
  message: string;
  client: ClientDto;
}
