# KubeFix Labs

Practice real-world Docker and Kubernetes troubleshooting in isolated, disposable environments.

## Architecture

- **Backend** (`backend/`) — Go 1.26 HTTP server with PostgreSQL, Google OAuth2 + JWT sessions, and Docker-backed challenge environments (stub unless built with the `docker` tag).
- **Frontend** (`frontend/`) — Next.js 15 App Router, React 19, Tailwind CSS.
- **Database** — PostgreSQL 16 with embedded SQL migrations.

## Run locally (without Docker Compose)

### Prerequisites
- Go 1.26+
- Node 20+ & npm
- PostgreSQL 16+
- A Google OAuth web client (only needed to test sign-in)

### 1. Start PostgreSQL and create the database

Make sure PostgreSQL is running, then create the application role and database:

```bash
sudo -u postgres psql -c "CREATE USER kubefix WITH PASSWORD 'kubefix';"
sudo -u postgres createdb -O kubefix kubefix
```

The backend runs the embedded schema migrations automatically on startup.

### 2. Configure and run the backend

In terminal 1, from the repository root:

```bash
cd backend
cp .env.example .env
go mod download
go run main.go
```

Edit `backend/.env` and set a random `JWT_SECRET` (at least 32 characters). For
Google sign-in, also set `GOOGLE_CLIENT_ID` and `GOOGLE_CLIENT_SECRET`.
`GOOGLE_REDIRECT_URL` must exactly match the authorized redirect URI registered
for your Google OAuth client; for local development use
`http://localhost:3000/auth/callback`.

### 3. Configure and run the frontend

In terminal 2, from the repository root:

```bash
cd frontend
npm ci
cp .env.example .env.local
npm run dev
```

For sign-in, edit `frontend/.env.local` and set `NEXT_PUBLIC_GOOGLE_CLIENT_ID`
to the same client ID as the backend. Set
`NEXT_PUBLIC_GOOGLE_REDIRECT_URI=http://localhost:3000/auth/callback`.
Open `http://localhost:3000`.

## Run with Docker Compose

Install and start Docker with the Compose plugin first. From the repository root,
create a `.env` file:

```bash
cat > .env <<'EOF'
JWT_SECRET=replace-with-a-random-secret-at-least-32-characters
GOOGLE_CLIENT_ID=your-google-client-id.apps.googleusercontent.com
GOOGLE_CLIENT_SECRET=your-google-client-secret
NEXT_PUBLIC_GOOGLE_CLIENT_ID=your-google-client-id.apps.googleusercontent.com
NEXT_PUBLIC_GOOGLE_REDIRECT_URI=http://localhost:3000/auth/callback
EOF

docker compose up -d --build
```

The frontend and backend can start without real Google credentials, but Google
sign-in will not work until valid credentials and the matching redirect URI are
configured. To view startup status and logs:

```bash
docker compose ps
docker compose logs -f backend frontend
```

Stop the services with `docker compose down`. To also delete the PostgreSQL
volume and all stored data, use `docker compose down -v`.

## Check that it is working

With the services running, check the backend health endpoint and public challenge
catalog:

```bash
curl -i http://localhost:8080/healthz
curl -i http://localhost:8080/api/challenges
```

The health endpoint should return HTTP `200` and `{"status":"ok"}`. The catalog
should return HTTP `200` and JSON containing a `challenges` array (it may be
empty until challenges have been added and published). Open
`http://localhost:3000` and confirm the page loads and displays the catalog.
To check the database container in Compose, run:

```bash
docker compose exec postgres pg_isready -U kubefix -d kubefix
```

To run code checks locally:

```bash
cd backend && go test ./... && go vet ./...
cd ../frontend && npm ci && npm run lint && npm test -- --runInBand && npm run build
```

The current default backend does not allocate real Docker or Kubernetes
challenge environments. It can serve the site, health endpoint, catalog, and
authentication API, but starting an environment needs the relevant environment
integration configured.

## Project structure

```
backend/        Go API server
frontend/       Next.js web app
docker-compose.yml  Multi-service orchestration
```

For Google sign-in, register `http://localhost:3000/auth/callback` as an authorized
redirect URI in Google Cloud and use that exact URL for both
`GOOGLE_REDIRECT_URL` and `NEXT_PUBLIC_GOOGLE_REDIRECT_URI`. If sign-in fails,
check that the client ID is the same in both apps, the redirect URI matches
exactly, and the backend logs do not report an authorization-code exchange error.

Quick start with Docker Compose

Create a .env file in the project root using the example in the README.
Run docker compose up -d --build.
Open http://localhost:3000.



docker compose ps
curl -i http://localhost:8080/healthz
curl -i http://localhost:8080/api/challenges
