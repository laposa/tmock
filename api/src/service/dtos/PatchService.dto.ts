import { IsOptional, IsString, Length } from 'class-validator';

export class PatchServiceDto {
  @IsString()
  @IsOptional()
  @Length(1, 255)
  name?: string;

  @IsString()
  @IsOptional()
  @Length(1, 2048)
  upstreamUrl?: string;

  @IsString()
  @IsOptional()
  @Length(1, 255)
  path?: string;
}
