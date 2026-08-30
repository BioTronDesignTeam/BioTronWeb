# BioTronStyle

Shared UI component library for BioTron frontends.

## Layout

| Path | What |
|---|---|
| `frontend/` | React + TypeScript component library and local preview |
| `.devcontainer/` | Node 22 development environment on Debian slim |
| `.env.example` | Root environment template for local preview settings |
| `docker-compose.yml` | Local component-preview container |

Copy `.env.example` to `.env` and run `npm --prefix frontend install` once. Then use
`npm --prefix frontend run dev` or `docker compose up --build` for the preview.
Build the consumable library with `npm --prefix frontend run build`.
