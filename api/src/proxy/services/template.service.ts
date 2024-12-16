import { Injectable } from '@nestjs/common';
import { Liquid } from 'liquidjs';

@Injectable()
export class TemplateService {
  protected engine: Liquid;

  constructor() {
    this.engine = new Liquid();
  }

  async parse(template: string): Promise<string> {
    if (template.startsWith('<% TEMPLATE: LiquidJS %>')) {
      try {
        const tpl = template.replace('<% TEMPLATE: LiquidJS %>', '');
        return await this.engine.parseAndRender(tpl);
      } catch (error) {
        return template;
      }
    }

    return template;
  }
}
