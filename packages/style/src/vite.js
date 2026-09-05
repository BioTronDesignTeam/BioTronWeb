import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

const faviconFile = fileURLToPath(new URL('./biotron-hand.png', import.meta.url));

/**
 * Serves the BioTron hand favicon during development and emits it into every
 * production bundle. Keep the matching link in index.html so the browser can
 * discover the icon before React starts.
 */
export function biotronFavicon(options = {}) {
  const fileName = options.fileName ?? 'biotron-hand.png';
  const publicPath = `/${fileName}`;
  let source;

  const getSource = () => {
    source ??= readFileSync(faviconFile);
    return source;
  };

  return {
    name: 'biotron-favicon',
    configureServer(server) {
      server.middlewares.use((request, response, next) => {
        const pathname = new URL(request.url ?? '/', 'http://localhost').pathname;
        if (pathname !== publicPath) {
          next();
          return;
        }

        response.statusCode = 200;
        response.setHeader('Content-Type', 'image/png');
        response.setHeader('Cache-Control', 'no-cache');
        response.end(getSource());
      });
    },
    generateBundle() {
      this.emitFile({
        type: 'asset',
        fileName,
        source: getSource(),
      });
    },
  };
}
