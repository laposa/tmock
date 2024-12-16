import { ClientDto as ClientDtoType } from 'database/schema';
import { ClientCondition } from '../client.interfaces';

// Manually create a class that mirrors the ClientDto type
export class ClientDto implements ClientDtoType {
  id: number;
  name: string;
	condition: ClientCondition;
	enabled: boolean;
}
