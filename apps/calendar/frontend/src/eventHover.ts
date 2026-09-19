import { createContext, useContext, type PointerEvent } from 'react';
import type { Occurrence } from './types';

/** The props an event chip spreads onto its button to get the hover card. */
export interface HoverBinding {
  onPointerEnter: (event: PointerEvent<HTMLElement>) => void;
  onPointerLeave: () => void;
  onFocus: (event: { currentTarget: HTMLElement }) => void;
  onBlur: () => void;
  'aria-describedby'?: string;
}

export const HoverContext = createContext<(occurrence: Occurrence) => HoverBinding>(() => {
  throw new Error('useEventHover needs an EventHoverProvider above it.');
});

/** Inside an EventHoverProvider: `{...hover(occurrence)}` on a chip's button gives it the details card. */
export function useEventHover() {
  return useContext(HoverContext);
}
