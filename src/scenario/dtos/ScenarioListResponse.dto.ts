import { ScenarioDto } from './';

export class ScenarioServiceDto {
  path: string;
  scenarios: ScenarioDto[];
}

export class ScenariosListResponse {
  services: ScenarioServiceDto[];
}
