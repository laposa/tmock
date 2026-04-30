CREATE TABLE "mp_users" (
	"id" serial PRIMARY KEY NOT NULL,
	"name" varchar NOT NULL,
	"email" varchar NOT NULL,
	"password" varchar NOT NULL,
	"admin" boolean DEFAULT false NOT NULL,
	"deleted" boolean DEFAULT false NOT NULL,
	CONSTRAINT "mp_users_email_unique" UNIQUE("email")
);
