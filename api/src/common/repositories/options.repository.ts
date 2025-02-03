import { Injectable } from '@nestjs/common';
import { AppDatabase, InjectDb } from '../providers/database.provider';

@Injectable()
export class OptionsRepository {
  constructor(@InjectDb() private db: AppDatabase) {}
}
