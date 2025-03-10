import {
  IsNumber,
  IsObject,
  IsOptional,
  IsString,
  Length,
} from 'class-validator';

export class PatchScenarioDto {
  @IsString()
  @IsOptional()
  @Length(1, 255)
  name?: string;

  @IsString()
  @IsOptional()
  @Length(1, 255)
  service: string;

  @IsString()
  @IsOptional()
  @Length(1, 255)
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
