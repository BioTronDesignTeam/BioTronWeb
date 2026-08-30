# BioTronStyle

Shared UI component library for BioTron frontends.

## Layout

| Path | What |
|---|---|
| `frontend/` | React + TypeScript component library and local preview |
| `.devcontainer/` | Node 22 development environment on Debian slim |

Reopen the repository in its devcontainer, then use
`npm --prefix frontend run dev` for the component preview. Build the consumable
library with `npm --prefix frontend run build`.

BioTronStyle is a build-time dependency for other frontends. It has no runtime
container, environment file, database, or production service.
