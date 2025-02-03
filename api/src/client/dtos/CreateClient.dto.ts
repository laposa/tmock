import {
  IsArray,
  IsBoolean,
  IsObject,
  IsOptional,
  IsString,
} from 'class-validator';

export class CreateClientDto {
  @IsString()
  name: string;

  @IsObject()
  @IsOptional()
  condition?: Record<string, string>;

  @IsArray()
  @IsOptional()
  scenarios?: number[];

  @IsBoolean()
  enabled: boolean;
}
