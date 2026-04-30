import {
  Controller,
  Get,
  Post,
  Body,
  Req,
  Res,
  Session,
  UseGuards,
  UsePipes,
  ValidationPipe,
} from '@nestjs/common';
import { Request, Response } from 'express';
import '@/types/express-session';
import { AuthService } from './auth.service';
import { LoginDto } from './dtos';
import { SessionAuthGuard } from '@/common/guards/session-auth.guard';
import { CsrfService } from '@/common/providers/csrf.service';

@Controller('auth')
export class AuthController {
  constructor(
    private readonly authService: AuthService,
    private readonly csrfService: CsrfService,
  ) {}

  @Post('/login')
  @UsePipes(ValidationPipe)
  async login(
    @Body() body: LoginDto,
    @Req() req: Request,
  ) {
    const user = await this.authService.validateUser(body.email, body.password);

    req.session.userId = user.id;
    req.session.isAdmin = user.admin;

    return { user };
  }

  @Post('/logout')
  async logout(@Req() req: Request, @Res() res: Response) {
    return new Promise<void>((resolve, reject) => {
      req.session.destroy((err) => {
        if (err) {
          reject(err);
          return;
        }
        res.clearCookie('connect.sid');
        res.clearCookie('__csrf');
        res.json({ message: 'Logged out' });
        resolve();
      });
    });
  }

  @Get('/me')
  @UseGuards(SessionAuthGuard)
  async me(@Session() session: Record<string, any>) {
    const user = await this.authService.getProfile(session.userId);
    return { user };
  }

  @Get('/csrf-token')
  getCsrfToken(@Req() req: Request, @Res() res: Response) {
    const token = this.csrfService.generateCsrfToken(req, res);
    res.json({ csrfToken: token });
  }
}
