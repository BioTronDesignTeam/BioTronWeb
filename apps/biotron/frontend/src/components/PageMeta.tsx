import { useEffect } from 'react';
import { NOT_FOUND_META, absoluteUrl, routeMeta } from '../data/seo';

interface PageMetaProps {
  /** A path from ROUTES in data/seo.ts. Leave it out on the not-found page. */
  path?: string;
}

const SITE_URL = import.meta.env.VITE_SITE_URL || window.location.origin;

function setMeta(selector: string, content: string) {
  document.querySelector<HTMLMetaElement>(selector)?.setAttribute('content', content);
}

/** Adds the tag when `value` is set and removes it when it is not. */
function setHeadTag(selector: string, create: () => HTMLElement, attribute: string, value?: string) {
  const existing = document.head.querySelector<HTMLElement>(selector);
  if (!value) {
    existing?.remove();
    return;
  }
  const tag = existing ?? document.head.appendChild(create());
  tag.setAttribute(attribute, value);
}

/**
 * Keeps the head in step with the page during in-app navigation. The first
 * load already has the right tags, because the build writes them into each
 * page's HTML. A page with no route is the not-found page: it gets no
 * canonical address and asks search engines not to index it.
 */
export default function PageMeta({ path }: PageMetaProps) {
  useEffect(() => {
    const route = path ? routeMeta(path) : undefined;
    const { title, description } = route ?? NOT_FOUND_META;
    const url = route ? absoluteUrl(SITE_URL, route.path) : undefined;

    document.title = title;
    setMeta('meta[name="description"]', description);
    setMeta('meta[property="og:title"]', title);
    setMeta('meta[property="og:description"]', description);
    setHeadTag('link[rel="canonical"]', () => Object.assign(document.createElement('link'), { rel: 'canonical' }), 'href', url);
    setHeadTag('meta[property="og:url"]', () => {
      const tag = document.createElement('meta');
      tag.setAttribute('property', 'og:url');
      return tag;
    }, 'content', url);
    setHeadTag('meta[name="robots"]', () => Object.assign(document.createElement('meta'), { name: 'robots' }), 'content', route ? undefined : 'noindex');
  }, [path]);

  return null;
}
