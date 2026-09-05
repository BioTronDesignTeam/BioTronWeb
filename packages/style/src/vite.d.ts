import type { Plugin } from 'vite';

export interface BioTronFaviconOptions {
  fileName?: string;
}

export declare function biotronFavicon(
  options?: BioTronFaviconOptions,
): Plugin;
