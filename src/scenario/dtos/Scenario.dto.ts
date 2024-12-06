import { ScenarioDto as ScenarioDtoType } from 'database/schema';

// Manually create a class that mirrors the ScenarioDto type
export class ScenarioDto implements ScenarioDtoType {
  id: number;
  name: string;
  service: string;
  requestMethod: string;
  requestPath: string;
  requestCondition: string;
  responseCode: number;
  responseHeaders: Record<string, string>;
  responseBody: string;
}
