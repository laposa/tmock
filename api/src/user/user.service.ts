import {
  BadRequestException,
  Injectable,
  NotFoundException,
} from '@nestjs/common';
import * as bcrypt from 'bcrypt';
import { UsersRepository } from '@/common/repositories/users.repository';
import { omitPassword } from '@/common/utils/user.utils';
import { CreateUserDto, UpdateUserDto, UpdateProfileDto } from './dtos';

@Injectable()
export class UserService {
  constructor(private usersRepository: UsersRepository) {}

  async getAll() {
    const users = await this.usersRepository.getAll();
    return users.map(omitPassword);
  }

  async getById(id: number) {
    const user = await this.usersRepository.getById(id);
    if (!user) {
      throw new NotFoundException(`User with id [${id}] was not found.`);
    }
    return user;
  }

  async getProfile(id: number) {
    const user = await this.getById(id);
    return omitPassword(user);
  }

  async create(data: CreateUserDto) {
    const existing = await this.usersRepository.getByEmail(data.email);
    if (existing) {
      throw new BadRequestException(`A user with email [${data.email}] already exists.`);
    }

    const hashedPassword = await bcrypt.hash(data.password, 10);
    const user = await this.usersRepository.create({
      ...data,
      admin: data.admin ?? false,
      password: hashedPassword,
    });
    return omitPassword(user);
  }

  async update(id: number, data: UpdateUserDto) {
    const user = await this.getById(id);

    if (data.email && data.email !== user.email) {
      const existing = await this.usersRepository.getByEmail(data.email);
      if (existing) {
        throw new BadRequestException(`A user with email [${data.email}] already exists.`);
      }
    }

    const updateData: Partial<{ name: string; email: string; password: string; admin: boolean }> = {};
    if (data.name !== undefined) updateData.name = data.name;
    if (data.email !== undefined) updateData.email = data.email;
    if (data.admin !== undefined) updateData.admin = data.admin;
    if (data.password) {
      updateData.password = await bcrypt.hash(data.password, 10);
    }

    const updated = await this.usersRepository.update(id, updateData);
    return omitPassword(updated!);
  }

  async updateProfile(userId: number, data: UpdateProfileDto) {
    const user = await this.getById(userId);

    if (data.email && data.email !== user.email) {
      const existing = await this.usersRepository.getByEmail(data.email);
      if (existing) {
        throw new BadRequestException(`A user with email [${data.email}] already exists.`);
      }
    }

    const updateData: Partial<{ name: string; email: string; password: string }> = {};
    if (data.name !== undefined) updateData.name = data.name;
    if (data.email !== undefined) updateData.email = data.email;

    if (data.newPassword) {
      if (!data.currentPassword) {
        throw new BadRequestException('Current password is required to set a new password.');
      }
      const isValid = await bcrypt.compare(data.currentPassword, user.password);
      if (!isValid) {
        throw new BadRequestException('Current password is incorrect.');
      }
      updateData.password = await bcrypt.hash(data.newPassword, 10);
    }

    const updated = await this.usersRepository.update(userId, updateData);
    return omitPassword(updated!);
  }

  async delete(id: number, currentUserId: number) {
    if (id === currentUserId) {
      throw new BadRequestException('You cannot delete your own account.');
    }

    const user = await this.getById(id);
    if (user.admin) {
      const adminCount = await this.usersRepository.countAdmins();
      if (adminCount <= 1) {
        throw new BadRequestException('Cannot delete the last admin user.');
      }
    }

    return this.usersRepository.softDelete(id);
  }
}
