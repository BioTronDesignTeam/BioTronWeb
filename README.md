# Biotron Site

Public BioTron team website built with React, TypeScript, Vite, and Three.js.
The application lives under `frontend/` so a future API can be added without
having to reorganize the repository.

## Layout

| Path | What |
|---|---|
| `frontend/src/` | React application source |
| `frontend/public/` | Static assets and 3D models |
| `frontend/Dockerfile` | Multi-stage static-site production image |
| `frontend/nginx.conf` | Nginx configuration for the static frontend container |
| `.devcontainer/` | Node 22 development environment on Debian slim |
| `.env.example` | Root environment template for local and deployed instances |
| `docker-compose.yml` | Local production-image runner |

Copy `.env.example` to `.env`. Then use `cd frontend && npm ci && npm run dev`
for local development, or `docker compose up --build` to run the
production-shaped container.
