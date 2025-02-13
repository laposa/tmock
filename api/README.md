# tmock

tmock API

## Installation

```bash
npm install
```

## Database migration

```bash
# generate migration
npm run database:generate

# run migration
npm run database:migrate
```

## Running the app

```bash
# development
npm run start

# watch mode
npm run start:dev

# production mode
npm run start:prod
```

## OpenAPI docs

Docs are available at `/swagger` endpoint.

## LiquidJS support

Scenarios responses can be templated using LiquidJS. In order to use it, the response body needs to start with `<% TEMPLATE: LiquidJS %>`.
For example:

```text
<% TEMPLATE: LiquidJS %>
{% assign seconds = 5 | times: 24 | times: 60 | times: 60 %}
{
  "validFrom": "{{ 'now' | date: "%s" | plus: seconds | date: "%Y-%m-%d" }}",
  // Other fields...
}
```
