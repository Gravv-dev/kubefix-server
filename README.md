# KubeFix Labs

Practice real-world Docker and Kubernetes troubleshooting in isolated, disposable environments.

## Architecture

- **Backend** (`backend/`) — Go 1.26 HTTP server with PostgreSQL, Google OAuth2 + JWT sessions, and Docker-backed challenge environments (stub unless built with the `docker` tag).
- **Frontend** (`frontend/`) — Next.js 15 App Router, React 19, Tailwind CSS.
- **Database** — PostgreSQL 16 with embedded SQL migrations.

## Run locally (without Docker)

### Prerequisites
- Go 1.26+
- Node 20+ & npm
- PostgreSQL

### 1. Database
```bash
sudo -u postgres psql -c "CREATE USER kubefix WITH PASSWORD 'kubefix' SUPERUSER;"
sudo -u postgres createdb -O kubefix kubefix
sudo -u postgres psql -d kubefix -c "CREATE EXTENSION IF NOT EXISTS pgcrypto;"
```

### 2. Backend
```bash
cd backend
cp .env.example .env
# edit .env: set JWT_SECRET, GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET
go mod download
go run main.go
# http://localhost:8080/healthz
```

### 3. Frontend
```bash
cd frontend
npm install
cp .env.example .env.local  # or create .env.local with NEXT_PUBLIC_* vars
npm run dev                 # http://localhost:3000
```

## Run with Docker Compose

```bash
# configure secrets
cat > .env <<'EOF'
JWT_SECRET=<32+ char random string>
GOOGLE_CLIENT_ID=<your Google OAuth client id>
GOOGLE_CLIENT_SECRET=<your Google OAuth client secret>
NEXT_PUBLIC_GOOGLE_CLIENT_ID=<your Google OAuth client id>
NEXT_PUBLIC_GOOGLE_REDIRECT_URI=http://localhost:3000/auth/callback
EOF

docker compose up -d --build
```

- Frontend: `http://localhost:3000`
- Backend API: `http://localhost:8080`
- Health: `http://localhost:8080/healthz`

## Project structure

```
backend/        Go API server
frontend/       Next.js web app
docker-compose.yml  Multi-service orchestration
```

## Known issues (to fix)

- `challenges.expected_answer` column is referenced in `store.go` but not created by migration `002`.
- Challenge list/detail scans read columns that aren't selected (column-count mismatch).
- Google OAuth callback route not wired end-to-end on the frontend.
