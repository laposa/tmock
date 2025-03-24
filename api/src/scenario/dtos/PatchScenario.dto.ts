import {
  IsNumber,
  IsObject,
  IsOptional,
  IsString,
  IsBoolean,
} from 'class-validator';

export class PatchScenarioDto {
  @IsString()
  @IsOptional()
  name?: string;

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

  @IsBoolean()
  @IsOptional()
  skipProxy?: boolean;
}
