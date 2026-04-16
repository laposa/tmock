import { IsString, Length } from 'class-validator';

export class CreateServiceDto {
  @IsString()
  @Length(1, 255)
  name: string;

  @IsString()
  @Length(1, 2048)
  upstreamUrl: string;

  @IsString()
  @Length(1, 255)
  path: string;
}
