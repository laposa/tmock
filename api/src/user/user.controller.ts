import {
  Body,
  Controller,
  Delete,
  Get,
  Param,
  Patch,
  Post,
  Session,
  UseGuards,
  UsePipes,
  ValidationPipe,
} from '@nestjs/common';
import { UserService } from './user.service';
import { CreateUserDto, UpdateUserDto, UpdateProfileDto } from './dtos';
import { SessionAuthGuard } from '@/common/guards/session-auth.guard';
import { AdminGuard } from '@/common/guards/admin.guard';

@Controller('user')
@UseGuards(SessionAuthGuard)
export class UserController {
  constructor(private readonly service: UserService) {}

  /**
   * Get current user's profile
   */
  @Get('/profile')
  async getProfile(@Session() session: Record<string, any>) {
    return this.service.getProfile(session.userId);
  }

  /**
   * Update current user's profile
   */
  @Patch('/profile')
  @UsePipes(ValidationPipe)
  async updateProfile(
    @Session() session: Record<string, any>,
    @Body() body: UpdateProfileDto,
  ) {
    return this.service.updateProfile(session.userId, body);
  }

  /**
   * List all users (admin only)
   */
  @Get('/')
  @UseGuards(AdminGuard)
  async getAll() {
    return this.service.getAll();
  }

  /**
   * Create a new user (admin only)
   */
  @Post('/')
  @UseGuards(AdminGuard)
  @UsePipes(ValidationPipe)
  async create(@Body() body: CreateUserDto) {
    const user = await this.service.create(body);
    return { message: 'User created', user };
  }

  /**
   * Update a user (admin only)
   */
  @Patch('/:id')
  @UseGuards(AdminGuard)
  @UsePipes(ValidationPipe)
  async update(@Param('id') id: string, @Body() body: UpdateUserDto) {
    const user = await this.service.update(+id, body);
    return { message: 'User updated', user };
  }

  /**
   * Delete a user (admin only)
   */
  @Delete('/:id')
  @UseGuards(AdminGuard)
  async delete(
    @Param('id') id: string,
    @Session() session: Record<string, any>,
  ) {
    await this.service.delete(+id, session.userId);
    return { message: 'User deleted' };
  }
}
