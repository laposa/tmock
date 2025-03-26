ALTER TABLE "mp_clients" ALTER COLUMN "enabled" SET NOT NULL;--> statement-breakpoint
ALTER TABLE "mp_scenarios" ADD COLUMN "skip_proxy" boolean DEFAULT false NOT NULL;