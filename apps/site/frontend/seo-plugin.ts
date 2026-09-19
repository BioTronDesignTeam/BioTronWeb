import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { dirname, join } from 'node:path';
import { loadEnv, type Plugin, type ResolvedConfig } from 'vite';
import { CONTACT } from './src/data/projects.ts';
import { NOT_FOUND_META, ROUTES, SITE_NAME, absoluteUrl, type RouteMeta } from './src/data/seo.ts';

/**
 * The site draws every page in the browser, so by itself the server would send
 * one index.html, with the home page's tags, for every address. Google runs
 * the JavaScript and copes. Link previews in Discord or iMessage do not, and
 * neither do most other crawlers.
 *
 * So the build writes a copy of index.html for each path in data/seo.ts, with
 * that page's own title, description, and canonical address in the head. It
 * also writes 404.html, sitemap.xml, robots.txt, and llms.txt from the same
 * list. nginx serves a path only if its file exists, which is what turns an
 * unknown address into a real 404.
 */

const START = '<!--seo:start-->';
const END = '<!--seo:end-->';
/** public/og.png. Rebuild it from og-image.html beside this file. */
const OG_IMAGE = { path: '/og.png', width: 1200, height: 630, alt: 'BioTron, the University of Waterloo biomechatronics design team' };

const escape = (value: string) => value.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

interface HeadOptions {
  siteUrl: string;
  title: string;
  description: string;
  /** Left out on the not-found page, which also asks not to be indexed. */
  path?: string;
  googleVerification?: string;
}

function headBlock({ siteUrl, title, description, path, googleVerification }: HeadOptions) {
  const url = path ? absoluteUrl(siteUrl, path) : undefined;
  const lines = [
    `<title>${escape(title)}</title>`,
    `<meta name="description" content="${escape(description)}" />`,
    url ? `<link rel="canonical" href="${escape(url)}" />` : '<meta name="robots" content="noindex" />',
    `<meta property="og:site_name" content="${SITE_NAME}" />`,
    '<meta property="og:type" content="website" />',
    url && `<meta property="og:url" content="${escape(url)}" />`,
    `<meta property="og:title" content="${escape(title)}" />`,
    `<meta property="og:description" content="${escape(description)}" />`,
    `<meta property="og:image" content="${escape(absoluteUrl(siteUrl, OG_IMAGE.path))}" />`,
    `<meta property="og:image:width" content="${OG_IMAGE.width}" />`,
    `<meta property="og:image:height" content="${OG_IMAGE.height}" />`,
    `<meta property="og:image:alt" content="${escape(OG_IMAGE.alt)}" />`,
    '<meta name="twitter:card" content="summary_large_image" />',
    googleVerification && path === '/' && `<meta name="google-site-verification" content="${escape(googleVerification)}" />`,
  ];
  return [START, ...lines.filter(Boolean), END].join('\n    ');
}

function sitemap(siteUrl: string, routes: RouteMeta[]) {
  const urls = routes.map((route) => `  <url><loc>${escape(absoluteUrl(siteUrl, route.path))}</loc></url>`);
  return `<?xml version="1.0" encoding="UTF-8"?>\n<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n${urls.join('\n')}\n</urlset>\n`;
}

function robots(siteUrl: string) {
  return `User-agent: *\nAllow: /\n\nSitemap: ${absoluteUrl(siteUrl, '/sitemap.xml')}\n`;
}

/** A plain-text guide to the site for language models, in the llms.txt layout: a title, a summary, then links. */
function llms(siteUrl: string, routes: RouteMeta[]) {
  const home = routes.find((route) => route.path === '/');
  const pages = routes
    .filter((route) => route.path !== '/')
    .map((route) => `- [${route.title.replace(/ \| BioTron$/, '')}](${absoluteUrl(siteUrl, route.path)}): ${route.description}`);
  return [
    `# ${SITE_NAME}`,
    '',
    `> ${home?.description ?? ''}`,
    '',
    'BioTron is the University of Waterloo biomechatronics design team, a student team. It welcomes every discipline and experience level.',
    '',
    '## Pages',
    '',
    ...pages,
    '',
    '## Contact',
    '',
    `- Email: ${CONTACT.email}`,
    `- Instagram: ${CONTACT.instagram}`,
    '',
  ].join('\n');
}

export function seoPages(): Plugin {
  let config: ResolvedConfig;
  let siteUrl = '';
  let googleVerification: string | undefined;

  return {
    name: 'biotron-seo-pages',
    configResolved(resolved) {
      config = resolved;
      const env = loadEnv(resolved.mode, resolved.envDir || resolved.root, 'VITE_');
      // The default only suits a local preview. A deployment must set the real address, or every canonical points at localhost.
      siteUrl = env.VITE_SITE_URL || 'http://localhost:5177';
      googleVerification = env.VITE_GOOGLE_SITE_VERIFICATION || undefined;
    },
    // index.html carries a marker in place of its head tags, in the dev server and the build alike.
    transformIndexHtml(html) {
      const home = ROUTES[0];
      return html.replace('<!--seo-->', headBlock({ siteUrl, ...home, googleVerification }));
    },
    async closeBundle() {
      if (config.command !== 'build') return;
      const outDir = join(config.root, config.build.outDir);
      const template = await readFile(join(outDir, 'index.html'), 'utf8');
      const block = new RegExp(`${START}[\\s\\S]*?${END}`);
      if (!block.test(template)) throw new Error('index.html lost its <!--seo--> marker, so no page would get its own tags.');
      const page = (options: Omit<HeadOptions, 'siteUrl' | 'googleVerification'>) =>
        template.replace(block, () => headBlock({ siteUrl, googleVerification, ...options }));

      const write = async (file: string, contents: string) => {
        await mkdir(dirname(join(outDir, file)), { recursive: true });
        await writeFile(join(outDir, file), contents);
      };
      for (const route of ROUTES.slice(1)) await write(`${route.path.slice(1)}/index.html`, page(route));
      await write('404.html', page(NOT_FOUND_META));
      await write('sitemap.xml', sitemap(siteUrl, ROUTES));
      await write('robots.txt', robots(siteUrl));
      await write('llms.txt', llms(siteUrl, ROUTES));
    },
  };
}
