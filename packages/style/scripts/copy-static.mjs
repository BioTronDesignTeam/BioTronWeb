import { copyFileSync } from 'node:fs';

copyFileSync('src/assets/biotron-hand.png', 'dist/biotron-hand.png');
copyFileSync('src/vite.js', 'dist/vite.js');
copyFileSync('src/vite.d.ts', 'dist/vite.d.ts');
