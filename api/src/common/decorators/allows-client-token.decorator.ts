import { SetMetadata } from '@nestjs/common';

export const ALLOWS_CLIENT_TOKEN = 'allowsClientToken';
export const AllowsClientToken = () => SetMetadata(ALLOWS_CLIENT_TOKEN, true);
