# BioTronStyle

Shared UI component library for BioTron frontends. It owns the BioTron palette,
brand assets, authentication surface, light/dark slider, account menu, and
button primitives without requiring Tailwind or another CSS framework.

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
import { AuthScreen, ThemeToggle, UserMenu } from '@biotron/style';
import '@biotron/style/styles.css';
```

## Theme bootstrap

Every frontend must keep the full theme bootstrap directly in its
`index.html`, before the application module. React runs too late to prevent a
white flash on a stored dark-mode visit. The bootstrap reads the shared
`biotron-theme` cookie first, migrates the legacy per-origin `darkMode` value,
toggles the `dark` class on `<html>`, sets `color-scheme`, and paints the
palette's dark or light page colour immediately.

The cookie is host-wide on `localhost`, so it crosses local development ports.
On BioTron hosts it is scoped to `biotron.ca` (and the current
`biotron-dev.com` development domain), so the preference crosses application
subdomains. `ThemeToggle` also checks the cookie while a page is open, allowing
already-open applications to follow a change made in another tab.

## Palette

- Ink: `#16033c`
- Deep: `#160b6c`
- Soft: `#aedbfc`
- Accent: `#3050b0`
- White: `#ffffff`
