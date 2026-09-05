# Site

Site is the public BioTron team website. It is frontend only: React, Vite,
and Three.js, served by Nginx. There is no backend and no database.

The pages are Projects, Sponsors, Calendar, and How to join. `/projects`
lists the current projects, then the past ones; `/past-projects` redirects
to `/projects#past`. Each project has a detail page with a 3D viewer.
`/calendar` shows the next public events from the Calendar API and links to
the Calendar app.

The homepage is a scroll-driven tour of a Three.js workshop room. The camera
stops at the About board, EXO, EMG Fabric, and e-NABLE, and a panel shows
each stop's content. Phones narrower than 640px, phones held in landscape,
browsers without WebGL, and reduced-motion visitors get the same content as
a stacked page instead. The 3D models are procedural placeholders;
`frontend/public/models/README.md` says how to replace them with GLB files.

This app was the `Biotron-Site` repository. The Compose project is still
`biotron-site`. The edge Nginx in `infra/` proxies to the alias `site-web`.

## Layout

| Path | What |
|------|------|
| `frontend/src/pages/` | One component per route. |
| `frontend/src/sections/` | Homepage sections: flythrough, stacked page, sponsors, join. |
| `frontend/src/three/` | Canvas, camera rig and path, workshop room, hotspots, models, detail viewer. |
| `frontend/src/components/` | Nav, footer, stop popup, buttons, animation helpers. |
| `frontend/src/lib/` | Lenis smooth scroll, GSAP, the scene store, hooks. |
| `frontend/src/data/` | Projects, past projects, sub-teams, stats, contact, sponsors. |
| `frontend/src/styles/` | `tokens.css`, `globals.css`, `ui.css`. |
| `frontend/public/models/` | Where GLB models go. Its README explains the swap. |
| `docker-compose.yml` | `site-web` on the `biotron` network. |
| `.env.example` | Every variable the app reads. Copy it to `.env`. |

## Run

Start `infra/` once, from the monorepo root, so that the `biotron` network
exists. Then the site:

```bash
docker compose -f infra/docker-compose.yml --env-file infra/.env up -d
cp apps/site/.env.example apps/site/.env
docker compose up -d --build site-web
```

The site is at http://localhost:5177, bound to `127.0.0.1`. The image builds
from the repository root so that it can see `packages/style`. The Calendar
page needs `calendar-api` on 8083 and `calendar-web` on 5176. Without them
the page shows its unavailable state; the rest of the site works.

## Develop

Open the monorepo in its devcontainer. It supplies Node 24, and its
post-create step installs the workspaces and builds `@biotron/style`. Then:

```bash
npm run dev -w apps/site/frontend      # http://localhost:5180
npm run build -w apps/site/frontend    # tsc, then vite build
npm run lint                           # Oxlint, from the root
```

The dev server takes 5180, not 5177, because the container keeps 5177 and
Vite runs with `strictPort`.

Verify a Three.js change in the rebuilt container, in a real browser. Scroll
to the stop the change touches and hold there; `three/CameraPath.ts` sets
where each stop dwells. Keep the hero's resting offset on
`.flythrough__hero-content`; GSAP animates `.flythrough__hero`, and a
transform on both makes the first scrolled pixel jump.

## Environment

One file, `apps/site/.env`, feeds Compose and Vite. `vite.config.ts` sets
`envDir` one folder up so that the dev server reads it too.

- `WEB_PORT` is the host port of `site-web`. Default `5177`.
- `VITE_CALENDAR_API_URL` is the Calendar API. The Calendar page fetches
  `/v1/events/upcoming?limit=5&days=42` from it, without credentials.
  Default `http://localhost:8083`.
- `VITE_CALENDAR_URL` is the Calendar app. The "Open full calendar" link
  goes there. Default `http://localhost:5176`.

The two `VITE_` values are compiled into the bundle. Compose passes them as
build arguments, so a change needs a rebuild.

## Design

The site has one theme, dark. `frontend/src/styles/tokens.css` is the
reference palette that the other apps' dark mode follows: page `#070b0e`,
surfaces `#0e151b` and `#131c24`, white text, `#9fb0ba` muted text.
`@biotron/style` carries the same values as its dark `--biotron-*` tokens.

Purple `#160b6c` is the highlight, as `--accent`. Blue `#3050b0` is
secondary, as `--accent-secondary`. Pale blue `#aedbfc` does not appear
anywhere in the site.

The header shows the wordmark from `@biotron/style` in its white form. The
favicon is the hand mark; the `biotronFavicon` Vite plugin serves it at
`/biotron-hand.png` and `index.html` links it.

Copy is active, concrete, and short. Do not use em dashes in site copy.

Contact channels live in `CONTACT` in `frontend/src/data/projects.ts`: the
email `biotron@uwaterloo.ca`, Instagram `@uwaterloo_biotron`, and the Discord
invite. The footer shows the email and Instagram; the Join page shows
Discord. There is no Facebook link. The Instagram mark is drawn in
`components/InstagramIcon.tsx` because `lucide-react` 1.x has no brand icons.
