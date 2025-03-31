import {
  IsArray,
  IsBoolean,
  IsObject,
  IsOptional,
  IsString,
  Length,
} from 'class-validator';

export class CreateClientDto {
  @IsString()
  @Length(1, 255)
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
