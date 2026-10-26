# BioTronStyle

Shared UI component library for BioTron frontends. It owns the BioTron palette,
brand assets, authentication surface, light/dark slider, account menu, and
button primitives without requiring Tailwind or another CSS framework.

## Layout

| Path | What |
|---|---|
| `src/` | Exported React components, types, and shared CSS |
| `.devcontainer/` | Node 24 development environment on Debian slim |
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

## Sessions

`src/auth` is the one sign-in flow for Auth, Exo, Calendar, and Sprinter.
Status and Site are public and must not mount it.

```tsx
<SessionProvider authUrl={AUTH_URL} appId="sprinter" permissions={['admin']}>
  <RequireAuth productName="Sprinter" permission="admin">{page}</RequireAuth>
</SessionProvider>
```

- `useSession()` gives `status` (`loading`, `anon`, `authed`, or
  `unavailable` when Auth cannot be reached), `operator`, `can(permission)`,
  `login`, `loginGuest`, `logout`, `refresh`, and `expire` (call it when an
  API answers 401).
- The provider reads Auth's `?auth=denied` and `?auth=banned` notices, strips
  them from the address, and says when a live session expires.
- `RequireAuth` shows loading, sign-in, the outage screen, or `AccessDenied`
  before the page. A public page skips it and gates only its editing controls
  on `can()`.
- `AccountControls` is the right end of a header: theme switch, then the
  account menu or a Sign in button.
- `authStrings` holds every user-facing word about signing in, roles, and
  permissions. `friendlyError` turns raw server refusals into those words.

## Theme bootstrap

Every frontend must keep the full theme bootstrap directly in its
`index.html`, before the application module. React runs too late to prevent a
white flash on a stored dark-mode visit. The bootstrap reads the shared
`biotron-theme` cookie, toggles the `dark` class on `<html>`, sets
`color-scheme`, and paints the palette's dark or light page colour
immediately.

The theme toggle writes the cookie for the last two labels of the page's
hostname, such as `uwbiotron.ca` for `calendar.uwbiotron.ca`, so the
preference crosses application subdomains. BioTron's domains never sit under
a multi-part suffix such as `co.uk`, so two labels is enough. On `localhost`,
any other name without a dot, and an IP address the cookie is host-only;
cookies ignore ports, so it still crosses local development ports.
`ThemeToggle` also checks the cookie while a page is open, allowing
already-open applications to follow a change made in another tab.

## Palette

- Ink: `#16033c`
- Deep: `#160b6c`
- Soft: `#aedbfc`
- Accent: `#3050b0`
- White: `#ffffff`
