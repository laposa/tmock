import { ClientCondition } from '@/client/client.interfaces';
import { relations } from 'drizzle-orm';
import {
  pgTable,
  serial,
  varchar,
  jsonb,
  boolean,
  smallint,
  primaryKey,
  integer,
} from 'drizzle-orm/pg-core';


const prefix = 'mp_v2_';

export const options = pgTable(`${prefix}options`, {
  key: varchar('key').primaryKey(),
  value: jsonb('value'),
});

export const services = pgTable(`${prefix}services`, {
  path: varchar('path').primaryKey(),
  upstreamUrl: varchar('upstream_url').notNull(),
  name: varchar('name').notNull(),
});

export const servicesRelations = relations(services, ({ many }) => ({
  scenarios: many(scenarios),
}));

export const scenarios = pgTable(`${prefix}scenarios`, {
  id: serial('id').primaryKey(),
  name: varchar('name').notNull(),
  service: varchar('service')
    .notNull()
    .references(() => services.path, {
      onDelete: 'cascade',
      onUpdate: 'cascade',
    }),
  requestMethod: varchar('request_method'),
  requestPath: varchar('request_path'),
  requestCondition: varchar('request_condition'),
  responseCode: smallint('response_code'),
  responseHeaders: jsonb('response_headers').$type<Record<string, string>>(),
  responseBody: varchar('response_body'),
});

export const scenariosRelations = relations(scenarios, ({ one }) => ({
  service: one(services, {
    fields: [scenarios.service],
    references: [services.path],
  }),
}));

export const clients = pgTable(`${prefix}clients`, {
  id: serial('id').primaryKey(),
  name: varchar('name').notNull(),
  enabled: boolean('enabled').default(true),
  condition: jsonb('condition').$type<ClientCondition>(),
});

export const clientsRelations = relations(clients, ({ many }) => ({
  scenarios: many(clientsScenarios),
}));

export const clientsScenarios = pgTable(
  `${prefix}clients_scenarios`,
  {
    clientId: integer('client_id')
      .notNull()
      .references(() => clients.id, {
        onDelete: 'cascade',
        onUpdate: 'cascade',
      }),
    scenarioId: integer('scenario_id')
      .notNull()
      .references(() => scenarios.id, {
        onDelete: 'cascade',
        onUpdate: 'cascade',
      }),
  },
  (t) => ({
    pk: primaryKey({ columns: [t.clientId, t.scenarioId] }),
  }),
);

export const clientsScenariosRelations = relations(clientsScenarios, ({ one }) => ({
  client: one(clients, {
    fields: [clientsScenarios.clientId],
    references: [clients.id],
  }),
  scenario: one(scenarios, {
    fields: [clientsScenarios.scenarioId],
    references: [scenarios.id],
  }),
}));


// type definitions
export type ScenarioDto = typeof scenarios.$inferSelect;
export type ServiceDto = typeof services.$inferSelect;
export type OptionsDto = typeof options.$inferSelect;
export type ClientDto = typeof clients.$inferSelect;

export type ServiceWithScenariosDto = ServiceDto & {
  scenarios: ScenarioDto[];
};

export type ClientWithScenariosDto = ClientDto & {
  scenarios: ScenarioDto[];
};
