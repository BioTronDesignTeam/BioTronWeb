// The extension is spelled out because the build config loads this file with Node, which needs it.
import { PROJECTS } from './projects.ts';

/**
 * Every page the site serves, with the title and description it shows to
 * search engines and link previews. Two readers share this list: PageMeta sets
 * the tags in the browser, and the build writes one HTML file for each path so
 * a crawler that runs no JavaScript still gets the right tags. Add a page here
 * and both follow. Keep this file free of asset imports, because the build
 * config loads it outside the bundler.
 */
export interface RouteMeta {
  path: string;
  title: string;
  description: string;
}

export const SITE_NAME = 'BioTron';

export const ROUTES: RouteMeta[] = [
  {
    path: '/',
    title: 'UW Biomechatronics Design Team | BioTron',
    description: 'We are a University of Waterloo student team building exoskeletons, EMG wearables, and assistive devices.',
  },
  {
    path: '/projects',
    title: 'Projects | BioTron',
    description: 'See Biotron’s current projects and the work that shaped them.',
  },
  ...PROJECTS.map((project) => ({
    path: `/projects/${project.slug}`,
    title: `${project.name} | BioTron`,
    description: project.blurb,
  })),
  {
    path: '/join',
    title: 'Join | BioTron',
    description: 'Join the Biotron Discord, come to any meeting, then finish your sub-team onboarding in Notion.',
  },
  {
    path: '/sponsors',
    title: 'Sponsors | BioTron',
    description: 'Meet the organizations that support Biotron or ask about a partnership.',
  },
  {
    path: '/calendar',
    title: 'Calendar | BioTron',
    description: 'See upcoming Biotron meetings and subscribe to team, project, or subteam calendars.',
  },
];

export const NOT_FOUND_META = {
  title: 'Page not found | BioTron',
  description: 'We could not find the page you requested.',
};

export function routeMeta(path: string) {
  return ROUTES.find((route) => route.path === path);
}

/** The public address of a path. The home page is the bare origin, with no trailing slash anywhere. */
export function absoluteUrl(siteUrl: string, path: string) {
  const origin = siteUrl.replace(/\/+$/, '');
  return path === '/' ? `${origin}/` : `${origin}${path}`;
}
