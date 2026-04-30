import { Injectable, UnauthorizedException } from '@nestjs/common';
import * as bcrypt from 'bcrypt';
import { UsersRepository } from '@/common/repositories/users.repository';
import { omitPassword } from '@/common/utils/user.utils';

@Injectable()
export class AuthService {
  constructor(private usersRepository: UsersRepository) {}

  async validateUser(email: string, password: string) {
    const user = await this.usersRepository.getByEmail(email);
    if (!user) {
      throw new UnauthorizedException('Invalid email or password');
    }

    const isValid = await bcrypt.compare(password, user.password);
    if (!isValid) {
      throw new UnauthorizedException('Invalid email or password');
    }

    return omitPassword(user);
  }

  async getProfile(userId: number) {
    const user = await this.usersRepository.getById(userId);
    if (!user) {
      throw new UnauthorizedException('User not found');
    }

    return omitPassword(user);
  }
}
