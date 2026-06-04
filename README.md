# T-Mock

T-Mock is a powerful HTTP mock and proxy service designed for testing and development. It intercepts HTTP requests destined for upstream services and returns predetermined mock responses based on configurable scenarios. Additionally, it can proxy requests to upstream services while applying transformations and templating.

**Key Use Cases:**
- Mock external API dependencies during development and testing
- Create multiple test scenarios for different business cases
- Implement conditional response routing based on request attributes
- Generate dynamic responses using templating
- Manage API responses across different test clients
- Reduce testing complexity by eliminating external service dependencies

## Table of Contents

- [Features](#features)
- [Project Structure](#project-structure)
- [Getting Started](#getting-started)
  - [Prerequisites](#prerequisites)
  - [Installation](#installation)
  - [Database Setup](#database-setup)
  - [Running the Application](#running-the-application)
- [Architecture](#architecture)
  - [Core Concepts](#core-concepts)
  - [System Overview](#system-overview)
- [API Documentation](#api-documentation)
- [Database Schema](#database-schema)
- [Configuration](#configuration)
- [Features Guide](#features-guide)
  - [Services](#services)
  - [Scenarios](#scenarios)
  - [Clients](#clients)
  - [LiquidJS Templating](#liquidjs-templating)
  - [Client Access Tokens](#client-access-tokens)
  - [Request Conditions](#request-conditions)
- [Development](#development)
- [Deployment](#deployment)
- [License](#license)


## Features

- **HTTP Proxy & Mock Server**: Route requests to mocked or real upstream services
- **Scenario-Based Mocking**: Define multiple test scenarios per service with different responses
- **Client Management**: Assign specific scenarios to different test clients for isolation
- **Conditional Response Routing**: Route requests based on headers, IP addresses, CIDR ranges, and complex conditions
- **Dynamic Templating**: Use LiquidJS templates in responses for dynamic data generation
- **Token-Based Authentication**: Secure programmatic access via client tokens for automated tests
- **Role-Based Access Control**: User authentication with admin and regular user roles
- **REST API**: Full REST API for programmatic management
- **Web Dashboard**: Intuitive Vue 3 frontend for managing services, scenarios, and clients
- **Response Caching**: Built-in caching for improved performance
- **OpenAPI/Swagger**: Interactive API documentation at `/swagger`
- **Docker Support**: Production-ready Docker image and Compose configuration
- **PostgreSQL Database**: Persistent storage with Drizzle ORM

## Project Structure

```
tmock/
├── api/                         # NestJS Backend API
│   ├── src/
│   │   ├── app.module.ts        # Root module
│   │   ├── app.controller.ts    # Health check endpoint
│   │   ├── app.config.ts        # Configuration
│   │   ├── auth/                # Authentication module
│   │   ├── client/              # Client management
│   │   ├── proxy/               # Proxy service and routing logic
│   │   ├── scenario/            # Scenario management
│   │   ├── service/             # Service management
│   │   ├── user/                # User management
│   │   ├── common/              # Shared utilities, decorators, guards, middleware
│   │   └── types/               # TypeScript type definitions
│   ├── database/
│   │   ├── schema.ts            # Drizzle ORM schema definitions
│   │   ├── seed.ts              # Database seeding script
│   │   ├── migrations/          # Database migrations
│   │   └── test-seed.sql        # Test data SQL
│   ├── package.json
│   └── README.md
├── app/                         # Vue 3 Frontend
│   ├── src/
│   │   ├── App.vue              # Root component
│   │   ├── main.ts              # Frontend entry point
│   │   ├── router.ts            # Vue Router configuration
│   │   ├── components/          # Vue components organized by feature
│   │   ├── views/               # Page views
│   │   ├── stores/              # Pinia state management
│   │   ├── apis/                # API client functions
│   │   ├── composables/         # Vue composables
│   │   ├── plugins/             # Vue plugins
│   │   └── assets/              # CSS and static assets
│   ├── index.html
│   ├── package.json
│   └── README.md
├── Dockerfile                    # Multi-stage Docker build
└── README.md                     # This file
```

## Getting Started

### Prerequisites

- **Node.js**: 18.x or higher
- **npm**: 9.x or higher
- **PostgreSQL**: 12.x or higher
- **Docker** (optional): For containerized deployment

### Installation

1. **Clone the repository:**
   ```bash
   git clone <repository_url>
   cd tmock
   ```

2. **Install dependencies for both API and app:**
   ```bash
   # Install API dependencies
   cd api
   npm install
   
   # In another terminal, install app dependencies
   cd app
   npm install
   ```

### Database Setup

T-Mock uses PostgreSQL with Drizzle ORM for database management.

1. **Create a PostgreSQL database:**
   ```bash
   createdb tmock
   ```

2. **Set up environment variables** (create `.env` in the `api/` directory):
   ```bash
   # Database Configuration
   DATABASE_HOST=localhost
   DATABASE_PORT=5432
   DATABASE_NAME=tmock
   DATABASE_USER=postgres
   DATABASE_PASSWORD=your_password
   
   # Server Configuration
   NODE_ENV=development
   PORT=3000
   
   # Session Configuration
   SESSION_SECRET=dev-secret-key-change-in-production
   
   # Optional: Proxy Configuration
   PROXY_REMOVE_FORWARDED_HEADERS=false
   SERVICES_API_KEY=your_api_key
   SERVICES_UPSTREAM_OVERRIDES=service-path::http://custom-url
   ```

3. **Run database migrations:**
   ```bash
   cd api
   npm run database:generate   # Generate migrations if needed
   npm run database:migrate    # Run migrations
   npm run database:seed       # (Optional) Seed with test data
   ```

### Running the Application

**Development Mode:**

Terminal 1 - Backend API:
```bash
cd api
npm run start:dev
```

Terminal 2 - Frontend:
```bash
cd app
npm run dev
```

The application will be available at:
- **Frontend**: http://localhost:5173
- **Backend API**: http://localhost:3000
- **Swagger/OpenAPI Docs**: http://localhost:3000/swagger

**Production Mode:**

```bash
cd api
npm run build
npm run start:prod
```

## Architecture

### Core Concepts

**Service**: Represents an upstream HTTP service that T-Mock can proxy to or mock. Each service has:
- A unique path identifier (e.g., `/api/users`)
- An upstream URL (the real service endpoint or mock URL)
- Associated scenarios for mocking responses

**Scenario**: A configurable mock response for a service. Scenarios define:
- Request matching criteria (method, path, conditions)
- Response status code and headers
- Response body (static or templated with LiquidJS)
- Whether to skip proxying and return the mock directly
- Assignment to specific clients

**Client**: A logical grouping of scenarios intended for specific testing scenarios. Clients can be:
- Test runners or test environments
- Specific testing phases
- Individual services or consumers
- Assigned specific scenarios to create isolated test conditions

**Request Condition**: Complex matching rules to determine scenario applicability:
- Header matching (exact or regex)
- IP address matching
- CIDR range matching
- Logical operators (AND, OR, NOT)

### System Overview

```
┌─────────────────────────────────────────────────────────────┐
│                     HTTP Client/Browser                     │
└──────────────────────────┬──────────────────────────────────┘
                           │ HTTP Request
                           ▼
┌─────────────────────────────────────────────────────────────┐
│                    T-Mock Frontend (Vue 3)                  │
│                   Configuration Dashboard                   │
│  - Manage Services, Scenarios, Clients, Users               │
│  - Interactive API Documentation (Swagger)                  │
└──────────────────────────┬──────────────────────────────────┘
                           │ REST API Calls
                           ▼
┌──────────────────────────────────────────────────────────────┐
│              T-Mock Backend (NestJS)                         │
│  ┌─────────────────────────────────────────────────────────┐ │
│  │  Proxy Service (http-proxy-middleware)                  │ │
│  │  - Route Request → Service                              │ │
│  │  - Match Request against Scenarios/Conditions           │ │
│  │  - Apply LiquidJS Templating                            │ │
│  │  - Return Mock or Forward to Upstream                   │ │
│  └─────────────────────────────────────────────────────────┘ │
│  ┌─────────────────────────────────────────────────────────┐ │
│  │  Management APIs                                        │ │
│  │  - Services API (CRUD)                                  │ │
│  │  - Scenarios API (CRUD)                                 │ │
│  │  - Clients API (CRUD + Token Management)                │ │
│  │  - Users API (Authentication + Authorization)           │ │
│  └─────────────────────────────────────────────────────────┘ │
└──────────────────────────┬───────────────────────────────────┘
                           │
                    ┌──────┴──────┐
                    ▼             ▼
          ┌──────────────────┐  ┌─────────────────────┐
          │  PostgreSQL DB   │  │ Upstream Services   │
          │ (Scenarios,      │  │ (Real APIs when     │
          │  Clients, Users) │  │  not mocking)       │
          └──────────────────┘  └─────────────────────┘
```

## API Documentation

Complete API documentation is available at the `/swagger` endpoint when the server is running.

Key API endpoints include:

### Services
- `GET /api/service` - List all services
- `POST /api/service` - Create a new service
- `GET /api/service/:path` - Get service details
- `PATCH /api/service/:path` - Update service
- `DELETE /api/service/:path` - Delete service

### Scenarios
- `GET /api/scenario` - List all scenarios
- `POST /api/scenario` - Create a new scenario
- `GET /api/scenario/:id` - Get scenario details
- `PATCH /api/scenario/:id` - Update scenario
- `DELETE /api/scenario/:id` - Delete scenario

### Clients
- `GET /api/client` - List all clients
- `POST /api/client` - Create a new client
- `GET /api/client/:id` - Get client details
- `PATCH /api/client/:id` - Update client
- `DELETE /api/client/:id` - Delete client
- `POST /api/client/:id/token` - Generate or rotate token
- `DELETE /api/client/:id/token` - Revoke token

### Users
- `GET /api/user` - List all users
- `POST /api/user` - Create a new user
- `PATCH /api/user/:id` - Update user
- `DELETE /api/user/:id` - Delete user

### Authentication
- `POST /api/auth/login` - Login and create session
- `POST /api/auth/logout` - Logout and destroy session

## Database Schema

The T-Mock database includes the following main tables:

### Services (`mp_services`)
Represents upstream services that T-Mock can proxy to.

| Column | Type | Description |
|--------|------|-------------|
| path | VARCHAR (PK) | Unique service identifier (e.g., "/api/users") |
| upstream_url | VARCHAR | URL of the upstream service |
| name | VARCHAR | Human-readable service name |

### Scenarios (`mp_scenarios`)
Mock response configurations for services.

| Column | Type | Description |
|--------|------|-------------|
| id | SERIAL (PK) | Unique scenario ID |
| name | VARCHAR | Scenario name |
| service | VARCHAR (FK) | Service this scenario belongs to |
| request_method | VARCHAR | HTTP method to match (GET, POST, etc.) |
| request_path | VARCHAR | Request path pattern to match |
| request_condition | VARCHAR | Complex matching condition (JSON) |
| response_code | SMALLINT | HTTP status code to return |
| response_headers | JSONB | Response headers map |
| response_body | VARCHAR | Response body (can include LiquidJS templates) |
| skip_proxy | BOOLEAN | If true, return mock; if false, proxy to upstream |

### Clients (`mp_clients`)
Test clients that can have specific scenarios assigned.

| Column | Type | Description |
|--------|------|-------------|
| id | SERIAL (PK) | Unique client ID |
| name | VARCHAR | Client name |
| enabled | BOOLEAN | Whether client is active |
| condition | JSONB | Condition for matching requests to this client |
| token | VARCHAR | Optional API token for programmatic access |

### Clients-Scenarios (`mp_clients_scenarios`)
Many-to-many mapping between clients and scenarios.

| Column | Type | Description |
|--------|------|-------------|
| client_id | INTEGER (FK) | Client ID |
| scenario_id | INTEGER (FK) | Scenario ID |

Primary key: (client_id, scenario_id)

### Users (`mp_users`)
System users for authentication and authorization.

| Column | Type | Description |
|--------|------|-------------|
| id | SERIAL (PK) | Unique user ID |
| name | VARCHAR | User's display name |
| email | VARCHAR (UNIQUE) | User's email (login credential) |
| password | VARCHAR | Bcrypt hashed password |
| admin | BOOLEAN | Whether user has admin privileges |
| deleted | BOOLEAN | Soft delete flag |

### Options (`mp_options`)
Key-value store for system settings.

| Column | Type | Description |
|--------|------|-------------|
| key | VARCHAR (PK) | Configuration key |
| value | JSONB | Configuration value |

## Configuration

### Environment Variables

**Database Configuration:**
```bash
DATABASE_HOST=localhost          # PostgreSQL host
DATABASE_PORT=5432               # PostgreSQL port
DATABASE_NAME=tmock              # Database name
DATABASE_USER=postgres           # Database user
DATABASE_PASSWORD=password       # Database password
DATABASE_ENABLE_SSL=false        # Enable SSL for DB connection
```

**Server Configuration:**
```bash
NODE_ENV=development             # Environment: development, production, test
PORT=3000                        # Server port
```

**Session Configuration:**
```bash
SESSION_SECRET=your-secret-key   # Session encryption key (REQUIRED in production)
```

**Proxy Configuration:**
```bash
PROXY_REMOVE_FORWARDED_HEADERS=false  # Remove X-Forwarded-* headers
SERVICES_API_KEY=your-api-key         # API key for upstream services
SERVICES_UPSTREAM_OVERRIDES=path::url # Override upstream URLs
```

#### Upstream Overrides Example

To override specific service URLs without changing the database:

```bash
SERVICES_UPSTREAM_OVERRIDES=/api/users::https://staging-api.example.com/users,/api/products::https://products-api.example.com
```

Format: `path::url,path::url,...`

### Cache Configuration

T-Mock includes a global cache manager. Disable caching for development:

```bash
DISABLE_CACHE=true
```

## Features Guide

### Services

Services represent the upstream endpoints that T-Mock manages. A service consists of:
- **Path**: Unique identifier (e.g., `/api/users`)
- **Name**: Human-readable identifier
- **Upstream URL**: The real service endpoint or mock destination

**Creating a Service:**
1. Navigate to the Services view
2. Click "Create Service"
3. Enter the path, name, and upstream URL
4. Click Save

### Scenarios

Scenarios define how T-Mock responds to requests. Each scenario can be configured to:
- Match specific request patterns (method, path)
- Match complex request conditions (headers, IP, etc.)
- Return a specific response (status, headers, body)
- Skip proxying and return the mock directly

**Creating a Scenario:**
1. Select a service
2. Click "Create Scenario"
3. Configure:
   - Name: Descriptive scenario name
   - Request Method: HTTP method (GET, POST, etc.)
   - Request Path: Path pattern to match
   - Request Condition: Complex nested conditions (optional)
   - Response Code: HTTP status code
   - Response Headers: Custom headers
   - Response Body: Static or templated content
   - Skip Proxy: Whether to return mock without proxying

### Clients

Clients allow you to assign specific scenarios to different testing contexts. A client can be:
- A test environment (staging, QA, development)
- A specific consumer service
- A test case or test suite

**Creating a Client:**
1. Navigate to Clients view
2. Click "Create Client"
3. Enter client name
4. Assign scenarios
5. Configure conditions (optional)
6. Click Save

### LiquidJS Templating

T-Mock supports dynamic response generation using LiquidJS templating language.

**Enabling Templates:**
To use templating in a response body, prefix the response with the template marker:

```
<% TEMPLATE: LiquidJS %>
```

**Example - Dynamic Date Generation:**
```
<% TEMPLATE: LiquidJS %>
{% assign seconds = 5 | times: 24 | times: 60 | times: 60 %}
{
  "id": 123,
  "name": "Product Name",
  "validFrom": "{{ 'now' | date: "%s" | plus: seconds | date: "%Y-%m-%d" }}",
  "createdAt": "{{ 'now' | date: "%Y-%m-%dT%H:%M:%SZ" }}"
}
```

**Example - Conditional Response:**
```
<% TEMPLATE: LiquidJS %>
{
  "status": "success",
  "timestamp": "{{ 'now' | date: "%Y-%m-%dT%H:%M:%SZ" }}",
  {% if premium %}
  "premium": true,
  "features": ["advanced", "api-access"]
  {% else %}
  "premium": false,
  "features": ["basic"]
  {% endif %}
}
```

**Common LiquidJS Filters:**
- `date`: Format dates
- `plus` / `minus`: Mathematical operations
- `times` / `divided_by`: Multiplication and division
- `upcase` / `downcase`: Case conversion
- `default`: Provide default values
- `first` / `last` / `join`: Array operations

For more LiquidJS documentation, visit: https://liquidjs.com/

### Client Access Tokens

Client Access Tokens enable programmatic access for automated tests without requiring user authentication. Each token belongs to a single client.

**Token Permissions:**
A token holder can only perform:
```http
PATCH /api/client/:clientId
```

**Request Headers:**
```http
X-Client-Token: {token_value}
```

**Allowed Request Body Fields:**
- `enabled` (boolean): Enable/disable the client
- `scenarios` (array): Assign scenarios to the client

**Example Usage:**
```bash
curl -X PATCH http://localhost:3000/api/client/1 \
  -H "X-Client-Token: abc123xyz..." \
  -H "Content-Type: application/json" \
  -d '{
    "enabled": true,
    "scenarios": [1, 2, 3]
  }'
```

**Managing Tokens:**
1. Navigate to Clients view
2. Find the client row
3. Click "View" under Tokens column (if token exists) or "(+)" to generate
4. In the modal:
   - Copy the token
   - Revoke the token (generates a new one)
   - Close the modal

### Request Conditions

Request conditions allow fine-grained control over when scenarios apply. Conditions support complex boolean logic and multiple matching criteria.

**Condition Types:**

**Header Matching:**
```json
{
  "headerMatch": {
    "header": "X-Client-ID",
    "value": "production"
  }
}
```

**Header Regex Matching:**
```json
{
  "headerRegex": {
    "header": "User-Agent",
    "value": ".*Chrome.*"
  }
}
```

**IP Address Matching:**
```json
{
  "ip": "192.168.1.100"
}
```

**CIDR Range Matching:**
```json
{
  "cidr": "192.168.1.0/24"
}
```

**Logical Operators:**

AND condition (all must match):
```json
{
  "and": [
    { "headerMatch": { "header": "X-Version", "value": "v2" } },
    { "ip": "192.168.1.100" }
  ]
}
```

OR condition (at least one must match):
```json
{
  "or": [
    { "ip": "192.168.1.100" },
    { "ip": "10.0.0.1" }
  ]
}
```

NOT condition (inverts logic):
```json
{
  "not": true,
  "headerMatch": { "header": "X-Debug", "value": "true" }
}
```

**Complex Example:**
```json
{
  "and": [
    { "headerMatch": { "header": "X-Tenant", "value": "customer-123" } },
    {
      "or": [
        { "cidr": "192.168.0.0/16" },
        { "headerRegex": { "header": "X-VPN", "value": "true" } }
      ]
    }
  ]
}
```

## Development

### Project Structure Details

**Backend (API):**
- Built with **NestJS** framework
- Modern TypeScript with strict mode
- Modular architecture with feature-based modules
- Uses **Drizzle ORM** for type-safe database queries
- **http-proxy-middleware** for request proxying
- **LiquidJS** for template rendering
- **bcrypt** for password hashing
- **csrf-csrf** for CSRF protection
- **express-session** for session management

**Frontend (App):**
- Built with **Vue 3** composition API
- **Vite** for fast development and optimized builds
- **Vue Router** for client-side routing
- **Pinia** for state management
- **Vuetify 3** for UI components
- **Axios** for HTTP requests
- **CodeMirror 6** for code editing

### Running Tests

Database seeding for development:
```bash
cd api
npm run database:seed
```

### Code Quality

**Linting:**
```bash
cd api
npm run lint

cd ../app
npm run lint
```

**Type Checking:**
```bash
cd app
npm run type-check
```

**Code Formatting:**
```bash
cd api
npm run format

cd ../app
npm run format
```

### Database Migrations

To create a new migration after modifying the schema:

```bash
cd api
npm run database:generate  # Generates migration files
npm run database:migrate   # Applies migrations
```

### Adding New Features

1. **Add database schema** in `api/database/schema.ts`
2. **Generate migration**: `npm run database:generate`
3. **Create module** in appropriate folder (`api/src/{feature}/`)
4. **Implement service layer** for business logic
5. **Add controller** for HTTP endpoints
6. **Register in AppModule** (`api/src/app.module.ts`)
7. **Add frontend components** in `app/src/components/`
8. **Add API client** in `app/src/apis/`

## Deployment

### Docker Deployment

Build the Docker image:
```bash
docker build -t tmock:latest .
```

The Dockerfile uses a multi-stage build:
1. Builds the frontend (Vue 3 app)
2. Builds the backend (NestJS API)
3. Serves frontend static files from the API

### Environment Setup for Production

Create a `.env` file with production-ready settings:

```bash
# CRITICAL: Must be set in production
SESSION_SECRET=your-very-secure-random-key-here

# Database
DATABASE_HOST=prod-db.example.com
DATABASE_PORT=5432
DATABASE_NAME=tmock_prod
DATABASE_USER=tmock_user
DATABASE_PASSWORD=strong-db-password
DATABASE_ENABLE_SSL=true

# Server
NODE_ENV=production
PORT=3000

# Proxy
PROXY_REMOVE_FORWARDED_HEADERS=true
```

### Health Checks

T-Mock includes a health check endpoint for monitoring:

```bash
GET /status
```

Returns:
```json
{ "msg": "ok" }
```

Use this endpoint in orchestration platforms (Kubernetes, Docker Compose, etc.).

## License

Copyright (c) 2024-2026, Laposa contributors
All rights reserved.

Redistribution and use in source and binary forms, with or without modification, are permitted provided that the following conditions are met:

1. Redistributions of source code must retain the above copyright notice, this list of conditions and the following disclaimer.

2. Redistributions in binary form must reproduce the above copyright notice, this list of conditions and the following disclaimer in the documentation and/or other materials provided with the distribution.

3. Neither the name of the copyright holder nor the names of its contributors may be used to endorse or promote products derived from this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
