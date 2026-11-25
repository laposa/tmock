-- 0001_init: the initial schema (spec §5.1).
-- Migrations are forward-only: never edit this file once released; add a new one instead.

CREATE TABLE services (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  path         text NOT NULL UNIQUE CHECK (path ~ '^[A-Za-z0-9._-]{1,255}$'),
  name         text NOT NULL CHECK (length(name) BETWEEN 1 AND 255),
  upstream_url text NOT NULL CHECK (length(upstream_url) BETWEEN 1 AND 2048)
);

CREATE TABLE scenarios (
  id                bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  service_id        bigint NOT NULL REFERENCES services(id) ON DELETE CASCADE,
  name              text NOT NULL CHECK (length(name) BETWEEN 1 AND 255),
  request_method    text,                     -- upper-case, NULL = any
  request_path      text,                     -- trimmed of '/', NULL = any
  request_condition text,                     -- expr source (§7.4), NULL = none
  response_code     integer CHECK (response_code BETWEEN 100 AND 599),
  response_headers  jsonb,                    -- {"Name": "value" | null}
  response_body     text,
  skip_proxy        boolean NOT NULL DEFAULT false,
  UNIQUE (service_id, name)
);

CREATE TABLE clients (
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name       text NOT NULL UNIQUE CHECK (length(name) BETWEEN 1 AND 255),
  enabled    boolean NOT NULL DEFAULT false,
  condition  jsonb,
  token_hash bytea UNIQUE                     -- SHA-256 of the token; NULL = no token
);

CREATE TABLE client_scenarios (
  client_id   bigint NOT NULL REFERENCES clients(id)   ON DELETE CASCADE,
  scenario_id bigint NOT NULL REFERENCES scenarios(id) ON DELETE CASCADE,
  PRIMARY KEY (client_id, scenario_id)
);

CREATE TABLE users (
  id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name          text NOT NULL CHECK (length(name) BETWEEN 1 AND 255),
  email         text NOT NULL,
  password_hash text NOT NULL,                -- §9.1
  admin         boolean NOT NULL DEFAULT false,
  created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_email_key ON users (lower(email));

CREATE TABLE sessions (
  id         bytea PRIMARY KEY,               -- HMAC-SHA256 of the cookie value (§9.2)
  user_id    bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at timestamptz NOT NULL
);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);
