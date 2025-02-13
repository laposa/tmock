CREATE TABLE IF NOT EXISTS "mp_clients" (
	"id" serial PRIMARY KEY NOT NULL,
	"name" varchar NOT NULL,
	"enabled" boolean DEFAULT true,
	"condition" jsonb
);
--> statement-breakpoint
CREATE TABLE IF NOT EXISTS "mp_clients_scenarios" (
	"client_id" integer NOT NULL,
	"scenario_id" integer NOT NULL,
	CONSTRAINT "mp_clients_scenarios_client_id_scenario_id_pk" PRIMARY KEY("client_id","scenario_id")
);
--> statement-breakpoint
CREATE TABLE IF NOT EXISTS "mp_options" (
	"key" varchar PRIMARY KEY NOT NULL,
	"value" jsonb
);
--> statement-breakpoint
CREATE TABLE IF NOT EXISTS "mp_scenarios" (
	"id" serial PRIMARY KEY NOT NULL,
	"name" varchar NOT NULL,
	"service" varchar NOT NULL,
	"request_method" varchar,
	"request_path" varchar,
	"request_condition" varchar,
	"response_code" smallint,
	"response_headers" jsonb,
	"response_body" varchar
);
--> statement-breakpoint
CREATE TABLE IF NOT EXISTS "mp_services" (
	"path" varchar PRIMARY KEY NOT NULL,
	"upstream_url" varchar NOT NULL,
	"name" varchar NOT NULL
);
--> statement-breakpoint
DO $$ BEGIN
 ALTER TABLE "mp_clients_scenarios" ADD CONSTRAINT "mp_clients_scenarios_client_id_mp_clients_id_fk" FOREIGN KEY ("client_id") REFERENCES "public"."mp_clients"("id") ON DELETE cascade ON UPDATE cascade;
EXCEPTION
 WHEN duplicate_object THEN null;
END $$;
--> statement-breakpoint
DO $$ BEGIN
 ALTER TABLE "mp_clients_scenarios" ADD CONSTRAINT "mp_clients_scenarios_scenario_id_mp_scenarios_id_fk" FOREIGN KEY ("scenario_id") REFERENCES "public"."mp_scenarios"("id") ON DELETE cascade ON UPDATE cascade;
EXCEPTION
 WHEN duplicate_object THEN null;
END $$;
--> statement-breakpoint
DO $$ BEGIN
 ALTER TABLE "mp_scenarios" ADD CONSTRAINT "mp_scenarios_service_mp_services_path_fk" FOREIGN KEY ("service") REFERENCES "public"."mp_services"("path") ON DELETE cascade ON UPDATE cascade;
EXCEPTION
 WHEN duplicate_object THEN null;
END $$;
