# Public site: search and link previews

This note covers `apps/site`. It records what the site does for search engines
and link previews, what a person still has to do, and what is left for later.

## How it works

The site draws every page in the browser. By itself the server would send one
`index.html`, with the home page's tags, for every address. Google runs the
JavaScript and copes. Link previews in Discord, iMessage, and Slack do not, and
neither do most other crawlers.

So the build writes one HTML file for each page:

- `src/data/seo.ts` lists every page with its title and description. It is the
  one place to add or change a page.
- `seo-plugin.ts` runs at the end of `vite build`. For each page it writes
  `dist/<path>/index.html` with that page's title, description, canonical
  address, and link preview tags. It also writes `404.html`, `sitemap.xml`,
  `robots.txt`, and `llms.txt` from the same list.
- `PageMeta` updates the same tags in the browser when a visitor moves between
  pages without a reload.
- `nginx.conf` serves a path only when its file exists. Any other address gets
  a real 404 status with the app shell, which draws the not-found page. `/join/`
  redirects to `/join`, so each page has one address.
- `public/og.png` is the picture a shared link shows. Its source is
  `og-image.html`, which holds the command that renders it.

## On a deployment

Set `VITE_SITE_URL` to the public address, with no trailing slash, before the
image is built. The default is `http://localhost:5177`. With the default, every
canonical tag, the sitemap, and the preview image address point at localhost,
and search engines ignore them.

## Needs a person: Google Search Console

Nobody has done this yet. It needs a Google account and the live domain.

1. Open Google Search Console and add the site as a property.
2. Verify it. With DNS access, use the DNS record and change nothing here.
   Without it, choose "HTML tag", copy the code, set it as
   `VITE_GOOGLE_SITE_VERIFICATION`, and rebuild. The build puts the tag on the
   home page.
3. Submit `https://<domain>/sitemap.xml` under Sitemaps.
4. After a few days, open URL Inspection for `/join` and `/projects/exo`. Check
   that the rendered page shows the real content and not a blank shell. This is
   the one check that proves Google reads the JavaScript pages.

Analytics is a separate decision. It needs a choice about visitor consent.

## Later

These are worth doing, and none blocks a launch.

### Load the 3D code only where it is used

This is the largest speed win. Every page downloads the 3D libraries: `/join`
was seen fetching `three`, `r3f`, and `gsap`. Only the home page's workshop
needs them at load. The project pages already load their viewer on demand.
Measured on the September 2026 build, before compression:

| File | Size |
|---|---|
| `three` | 708 KB |
| the app | 427 KB |
| `r3f`, the React 3D layer | 320 KB |
| `gsap` | 115 KB |

Load the home page's workshop with `React.lazy`, as `ProjectDetail` does for
its viewer, so `/join`, `/sponsors`, and `/calendar` skip those files. Measure before and after with Lighthouse on
`/join`. This touches routing and the loading screen, so treat it as its own
piece of work.

### Shrink the ODrive logo

`src/assets/sponsors/odrive.webp` is 177 KB and 2500 pixels wide. The page
shows it about 200 pixels wide. Scale it to about 800 pixels. Do not recolour
it: see the comment in `src/data/sponsors.ts`.

### Add an Organization block

One small `application/ld+json` block on the home page, with the team name,
the logo, the site address, and the Instagram link. It helps a search engine
show the right name and logo. The site needs no other structured data.

### Check for broken links before a launch

The Discord and Notion invites in `src/data/projects.ts` are the links most
likely to expire. Open each external link once before a launch, and again each
term.

### Trim two long descriptions

Search results cut a description at about 155 characters. The EMG Fabric blurb
is 158 and the e-NABLE blurb is 162, so each may lose its last word or two. The
same text shows on the project pages, so change it with care.

## Checked and not needed

- **No `noindex` tags** on any real page. Only the not-found page has one.
- **One `h1` on every page**, and no skipped heading levels.
- **Every image has alt text.**
- **Addresses are clean:** `/projects`, `/join`, `/sponsors`, `/calendar`.
- **Breadcrumbs:** the site is two levels deep, so they add nothing.
- **Internal links:** the navigation and the footer reach every page.
- **A backlink campaign:** ask the university, the sponsors, and competition
  pages for a link. That is enough for a student team.
