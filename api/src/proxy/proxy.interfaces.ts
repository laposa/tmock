import { ClientWithScenariosDto, ScenarioDto } from 'database/schema';
import { IncomingMessage, ServerResponse } from 'http';

export interface RequestScenario {
  scenario?: ScenarioDto;
  client?: ClientWithScenariosDto;
}

export type ProxyResponse = ServerResponse<IncomingMessage> & {
  startTime?: number;
  locals?: {
    mock: RequestScenario;
  };
};
