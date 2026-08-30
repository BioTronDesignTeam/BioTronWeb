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
for local development, or `docker compose up -d --build` to keep the
production-shaped container running.

The container is available at `http://127.0.0.1:18083` by default. That port is
reserved for the public site after the local OAuth, Exo, and Logger ports ending
at `18082`; override `WEB_PORT` in `.env` if the workspace map changes. The
container also joins the shared `biotron` network as `site-web`, which is the
stable alias used by the Server Nginx edge.
