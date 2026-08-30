# Biotron Site

Public BioTron team website built with React, TypeScript, Vite, and Three.js.

This repository is intentionally frontend-only, so the Vite application stays
at the repository root rather than adding empty `backend/` or `prisma/`
directories.

## Layout

| Path | What |
|---|---|
| `src/` | React application source |
| `public/` | Static assets and 3D models |
| `archive/` | Previous site retained during the refactor |
| `.devcontainer/` | Node 22 development environment on Debian slim |
| `Dockerfile` | Multi-stage static-site production image |
| `docker-compose.yml` | Local production-image runner |

Use `npm ci && npm run dev` for local development, or
`docker compose up --build` to run the production-shaped container.
