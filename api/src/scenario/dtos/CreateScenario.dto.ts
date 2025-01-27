import {
  IsNumber,
  IsObject,
  IsOptional,
  IsString,
} from 'class-validator';

export class CreateScenarioDto {
  @IsString()
  name: string;

  @IsString()
  service: string;

  @IsString()
  @IsOptional()
  requestMethod?: string;

  @IsString()
  @IsOptional()
  requestPath?: string;

  @IsString()
  @IsOptional()
  requestCondition?: string;

  @IsNumber()
  @IsOptional()
  responseCode?: number;

  @IsObject()
  @IsOptional()
  responseHeaders?: Record<string, string>;

  @IsString()
  @IsOptional()
  responseBody?: string;
}
