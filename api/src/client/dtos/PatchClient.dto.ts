import {
  IsArray,
  IsBoolean,
  IsIP,
  IsOptional,
  IsString,
  Length,
  ValidateNested,
} from 'class-validator';
import { ClientCondition } from '../client.interfaces';
import { Type } from 'class-transformer';

export class ClientConditionHeaderDto {
  @IsString()
  @Length(1, 255)
  header: string;

  @IsString()
  @Length(1, 255)
  value: string;
}

export class ClientConditionDto implements ClientCondition {
  @IsArray()
  @IsOptional()
  @ValidateNested({ each: true })
  @Type(() => ClientConditionDto)
  and?: ClientConditionDto[];

  @IsArray()
  @IsOptional()
  @ValidateNested({ each: true })
  @Type(() => ClientConditionDto)
  or?: ClientConditionDto[];

  @IsBoolean()
  @IsOptional()
  not?: boolean;

  @IsOptional()
  @ValidateNested()
  @Type(() => ClientConditionHeaderDto)
  headerMatch?: ClientConditionHeaderDto;

  @IsOptional()
  @ValidateNested()
  @Type(() => ClientConditionHeaderDto)
  headerRegex?: ClientConditionHeaderDto;

  @IsString()
  @IsOptional()
  @IsIP()
  ip?: string;

  @IsString()
  @IsOptional()
  @Length(1, 255)
  cidr?: string;
}

export class PatchClientDto {
  @IsString()
  @IsOptional()
  @Length(1, 255)
  name?: string;

  @IsOptional()
  @ValidateNested()
  @Type(() => ClientConditionDto)
  condition?: ClientConditionDto;

  @IsArray()
  @IsOptional()
  scenarios?: number[];

  @IsBoolean()
  @IsOptional()
  enabled?: boolean;
}
