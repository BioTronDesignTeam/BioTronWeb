# BioTronStyle

Shared UI component library for BioTron frontends.

## Layout

| Path | What |
|---|---|
| `src/` | Exported React components, types, and shared CSS |
| `.devcontainer/` | Node 22 development environment on Debian slim |
| `package.json` | Library exports and package metadata |

Reopen the repository in its devcontainer, then use
`npm run dev` for the component preview. Build the consumable library with
`npm run build`.

BioTronStyle is a build-time dependency for other frontends. It has no runtime
container, environment file, database, or production service.

Consumers install a tagged release or pinned commit from GitHub, then import
the components and stylesheet:

```bash
npm install github:BioTronDesignTeam/BioTronStyle#<tag-or-commit>
```

```tsx
import { Button } from '@biotron/style';
import '@biotron/style/styles.css';
```
