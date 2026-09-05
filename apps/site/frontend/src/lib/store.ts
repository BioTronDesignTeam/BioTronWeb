import { create } from 'zustand';
import type { ModelId } from '../data/projects';

/** Ordered stops along the Home camera path. */
export type Stop = 'about' | ModelId;
export const STOP_ORDER: Stop[] = ['about', 'exo', 'emg', 'enable'];

interface SceneState {
  /** Normalized progress (0..1) through the Home fly-through section. */
  progress: number;
  /** The stop the camera is currently nearest / dwelling at. */
  activeStop: Stop | null;
  /** Whether the heavy 3D scene has finished its first paint. */
  ready: boolean;
  /** Pointer in normalized device coords (-1..1) for subtle parallax. */
  pointer: { x: number; y: number };

  setProgress: (p: number) => void;
  setActiveStop: (s: Stop | null) => void;
  setReady: (r: boolean) => void;
  setPointer: (x: number, y: number) => void;
}

export const useScene = create<SceneState>((set) => ({
  progress: 0,
  activeStop: 'about',
  ready: false,
  pointer: { x: 0, y: 0 },

  setProgress: (progress) => set({ progress }),
  setActiveStop: (activeStop) => set((s) => (s.activeStop === activeStop ? s : { activeStop })),
  setReady: (ready) => set({ ready }),
  setPointer: (x, y) => set({ pointer: { x, y } }),
}));
