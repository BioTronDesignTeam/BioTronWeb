import { Vector3 } from 'three';
import type { Stop } from '../lib/store';

export interface Keyframe {
  /** Progress (0..1) through the fly-through at which this pose is reached. */
  at: number;
  /** Camera world position. */
  pos: [number, number, number];
  /** Point the camera looks at. */
  target: [number, number, number];
  /** The stop this keyframe represents, if it is a dwell point. */
  stop?: Stop;
}

/**
 * Ordered camera keyframes for the Home fly-through around the workshop room.
 * The room sits roughly centered at origin with project models around it
 * (see PROJECTS[].anchor in data/projects.ts). Establishing + exit frames pull
 * back to reveal the whole room.
 */
export const KEYFRAMES: Keyframe[] = [
  // Establishing shot: isometric pull-back over the whole room.
  { at: 0.0, pos: [7.5, 6.5, 9.5], target: [0, 0.8, -1.5] },
  // About board (mounted on the back-left wall).
  { at: 0.14, pos: [-3.2, 2.2, 3.6], target: [-5.2, 1.8, -3.2], stop: 'about' },
  // EXO (left bench).
  { at: 0.4, pos: [-2.0, 1.5, 2.2], target: [-4.2, 0.6, -1.5], stop: 'exo' },
  // EMG Fabric (center-back bench).
  { at: 0.64, pos: [0, 1.7, 1.2], target: [0, 0.7, -3.4], stop: 'emg' },
  // e-NABLE (right bench).
  { at: 0.88, pos: [2.0, 1.5, 2.2], target: [4.2, 0.5, -1.5], stop: 'enable' },
  // Exit shot: pull back out before the page leaves the room.
  { at: 1.0, pos: [6.5, 5.5, 10], target: [0, 0.6, -2] },
];

/** Progress value at which each stop's dwell is centered. */
export const STOP_AT: Record<Stop, number> = {
  about: 0.14,
  exo: 0.4,
  emg: 0.64,
  enable: 0.88,
};

/** Half-width of the progress window in which a stop's popup is shown. */
export const STOP_WINDOW = 0.11;

const _a = new Vector3();
const _b = new Vector3();

/** Interpolated camera pose at a given progress, with eased segments. */
export function sampleCamera(progress: number, outPos: Vector3, outTarget: Vector3) {
  const p = Math.min(1, Math.max(0, progress));
  let i = 0;
  while (i < KEYFRAMES.length - 1 && KEYFRAMES[i + 1].at < p) i++;
  const a = KEYFRAMES[i];
  const b = KEYFRAMES[Math.min(i + 1, KEYFRAMES.length - 1)];
  const span = b.at - a.at || 1;
  const tRaw = (p - a.at) / span;
  // smoothstep for natural slow-in / slow-out (the "dwell" feel).
  const t = tRaw * tRaw * (3 - 2 * tRaw);

  outPos.copy(_a.set(...a.pos)).lerp(_b.set(...b.pos), t);
  outTarget.copy(_a.set(...a.target)).lerp(_b.set(...b.target), t);
}

/** Which stop (if any) is active at a given progress. */
export function stopAtProgress(progress: number): Stop | null {
  let best: Stop | null = null;
  let bestDist = STOP_WINDOW;
  (Object.keys(STOP_AT) as Stop[]).forEach((stop) => {
    const d = Math.abs(progress - STOP_AT[stop]);
    if (d < bestDist) {
      bestDist = d;
      best = stop;
    }
  });
  return best;
}
