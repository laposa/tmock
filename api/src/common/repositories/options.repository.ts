import { eq } from 'drizzle-orm';
import { Injectable } from '@nestjs/common';
import { options } from 'database/schema';
import { AppDatabase, InjectDb } from '../providers/database.provider';

@Injectable()
export class OptionsRepository {
  constructor(@InjectDb() private db: AppDatabase) {}
}
