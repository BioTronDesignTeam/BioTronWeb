# BioTronStyle

Shared UI component library for BioTron frontends.

## Layout

| Path | What |
|---|---|
| `frontend/` | React + TypeScript component library and local preview |
| `.devcontainer/` | Node 22 development environment on Debian slim |
| `docker-compose.yml` | Local component-preview container |

Run `npm --prefix frontend install` once, then use
`npm --prefix frontend run dev` or `docker compose up --build` for the preview.
Build the consumable library with `npm --prefix frontend run build`.
