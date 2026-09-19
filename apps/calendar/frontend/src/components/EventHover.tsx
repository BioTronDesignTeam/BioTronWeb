import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { fullDateTimeLabel } from '../date';
import { HoverContext, type HoverBinding } from '../eventHover';
import type { Occurrence } from '../types';

interface HoverTarget {
  occurrence: Occurrence;
  anchor: DOMRect;
}

/** A pause before the card opens, so sweeping the pointer across a busy month does not flash a card per event. */
const OPEN_DELAY = 250;
const CARD_ID = 'event-hover-card';
const GAP = 8;

/**
 * Draws one details card for whichever event chip the pointer rests on or the
 * keyboard focuses. The card is read-only and ignores the pointer, so it never
 * gets between the pointer and the chip; a click still opens the full details.
 */
export function EventHoverProvider({ children }: { children: ReactNode }) {
  const [target, setTarget] = useState<HoverTarget>();
  const timer = useRef<number>(undefined);

  const hide = useCallback(() => {
    window.clearTimeout(timer.current);
    setTarget(undefined);
  }, []);

  const show = useCallback((occurrence: Occurrence, element: HTMLElement, delay: number) => {
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => setTarget({ occurrence, anchor: element.getBoundingClientRect() }), delay);
  }, []);

  useEffect(() => () => window.clearTimeout(timer.current), []);

  // The card is pinned to where the chip was. Once anything scrolls or a click
  // lands, that place is stale, and a click is about to open the details anyway.
  useEffect(() => {
    if (!target) return;
    window.addEventListener('scroll', hide, true);
    window.addEventListener('pointerdown', hide, true);
    window.addEventListener('keydown', hide, true);
    return () => {
      window.removeEventListener('scroll', hide, true);
      window.removeEventListener('pointerdown', hide, true);
      window.removeEventListener('keydown', hide, true);
    };
  }, [target, hide]);

  const bind = useMemo(() => (occurrence: Occurrence): HoverBinding => ({
    // Touch has no hover: a finger lands as pointerenter and then a click, and
    // the card would flash under the details it is about to open.
    onPointerEnter: (event) => { if (event.pointerType === 'mouse') show(occurrence, event.currentTarget, OPEN_DELAY); },
    onPointerLeave: hide,
    onFocus: (event) => { if (event.currentTarget.matches(':focus-visible')) show(occurrence, event.currentTarget, 0); },
    onBlur: hide,
    'aria-describedby': target?.occurrence === occurrence ? CARD_ID : undefined,
  }), [show, hide, target]);

  return (
    <HoverContext.Provider value={bind}>
      {children}
      {target && createPortal(<HoverCard target={target} />, document.body)}
    </HoverContext.Provider>
  );
}

function HoverCard({ target }: { target: HoverTarget }) {
  const { occurrence, anchor } = target;
  const card = useRef<HTMLDivElement>(null);
  const [place, setPlace] = useState<{ top: number; left: number }>();

  // Beside the chip: to its right when there is room, else to its left, and
  // never past an edge of the window. It is measured before it is shown.
  useLayoutEffect(() => {
    const box = card.current?.getBoundingClientRect();
    if (!box) return;
    const right = anchor.right + GAP;
    const left = right + box.width <= window.innerWidth - GAP ? right : anchor.left - GAP - box.width;
    setPlace({
      left: Math.max(GAP, Math.min(left, window.innerWidth - GAP - box.width)),
      top: Math.max(GAP, Math.min(anchor.top, window.innerHeight - GAP - box.height)),
    });
  }, [anchor]);

  return (
    <div
      ref={card}
      id={CARD_ID}
      role="tooltip"
      style={{ top: place?.top ?? 0, left: place?.left ?? 0, visibility: place ? 'visible' : 'hidden' }}
      className="pointer-events-none fixed z-40 max-h-[calc(100dvh-1rem)] w-72 max-w-[calc(100vw-1rem)] overflow-hidden rounded-2xl border border-ink/10 bg-white p-4 text-ink shadow-xl dark:border-line-strong dark:bg-surface dark:text-white"
    >
      {/* The card cannot be scrolled, so every field that can run long is cut to fit. The full text is one click away. */}
      <p className="line-clamp-3 break-words font-semibold leading-snug">{occurrence.title}</p>
      <p className="mt-1 text-sm text-ink/70 dark:text-muted">{fullDateTimeLabel(occurrence)}</p>
      <div className="mt-3 flex flex-wrap gap-1.5">
        <span className="rounded-full bg-brand/10 px-2.5 py-0.5 text-xs font-semibold text-brand dark:bg-brand/25 dark:text-link">{occurrence.scope_path}</span>
        {occurrence.recurring && <span className="rounded-full bg-deep/10 px-2.5 py-0.5 text-xs font-semibold text-deep dark:bg-white/10 dark:text-white">Weekly</span>}
      </div>
      {occurrence.location && <p className="mt-3 line-clamp-3 break-words text-sm">{occurrence.location}</p>}
      {occurrence.description && <p className="mt-2 line-clamp-4 whitespace-pre-wrap break-words text-sm text-ink/70 dark:text-muted">{occurrence.description}</p>}
    </div>
  );
}
