import { ScenarioDto as ScenarioDtoType } from 'database/schema';

// Manually create a class that mirrors the ScenarioDto type
export class ScenarioDto implements ScenarioDtoType {
  id: number;
  name: string;
  service: string;
  requestMethod: string | null;
  requestPath: string | null;
  requestCondition: string | null;
  responseCode: number | null;
  responseHeaders: Record<string, string> | null;
  responseBody: string | null;
}
