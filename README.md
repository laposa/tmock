# tmock

Testing Mock Proxy

## Installation

```bash
yarn install
```

## Database migration

```bash
# generate migration
yarn database:generate

# run migration
yarn database:migrate
```

## Running the app

```bash
# development
yarn start

# watch mode
yarn start:dev

# production mode
yarn start:prod
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
