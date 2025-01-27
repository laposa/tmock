import {
  IsArray,
  IsBoolean,
  IsObject,
  IsOptional,
  IsString,
} from 'class-validator';
import { ClientCondition } from '../client.interfaces';

export class PatchClientDto {
  @IsString()
  @IsOptional()
  name?: string;

  @IsObject()
  @IsOptional()
  condition?: ClientCondition;

  @IsArray()
  @IsOptional()
  scenarios?: number[];

  @IsBoolean()
  @IsOptional()
  enabled?: boolean;
}
