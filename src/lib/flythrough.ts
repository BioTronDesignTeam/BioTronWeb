import type { Stop } from './store';
import { STOP_AT } from '../three/CameraPath';
import { scrollToY } from './SmoothScroll';

/** Scroll range (in px) covered by the pinned Home fly-through section. */
let range: { start: number; end: number } | null = null;

export function registerFlythroughRange(start: number, end: number) {
  range = { start, end };
}

export function clearFlythroughRange() {
  range = null;
}

/** Jump the page scroll so the camera lands on a given stop. */
export function jumpToStop(stop: Stop) {
  if (!range) return;
  const progress = STOP_AT[stop];
  const y = range.start + progress * (range.end - range.start);
  scrollToY(y, { duration: 1.4 });
}

export function hasFlythrough() {
  return range !== null;
}
