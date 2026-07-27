import { useEffect, useRef } from 'react';
import { gsap } from '../lib/gsap';
import { useHasFinePointer, useReducedMotion } from '../lib/hooks';

/**
 * Custom trailing cursor dot + ring. Only rendered for fine pointers and when
 * reduced motion is off — touch users keep the native experience.
 */
export default function Cursor() {
  const dot = useRef<HTMLDivElement>(null);
  const ring = useRef<HTMLDivElement>(null);
  const fine = useHasFinePointer();
  const reduced = useReducedMotion();

  useEffect(() => {
    if (!fine || reduced) return;
    const xDot = gsap.quickTo(dot.current, 'x', { duration: 0.12, ease: 'power3' });
    const yDot = gsap.quickTo(dot.current, 'y', { duration: 0.12, ease: 'power3' });
    const xRing = gsap.quickTo(ring.current, 'x', { duration: 0.4, ease: 'power3' });
    const yRing = gsap.quickTo(ring.current, 'y', { duration: 0.4, ease: 'power3' });

    const move = (e: PointerEvent) => {
      xDot(e.clientX);
      yDot(e.clientY);
      xRing(e.clientX);
      yRing(e.clientY);
    };
    const over = (e: PointerEvent) => {
      const interactive = (e.target as HTMLElement)?.closest('a,button,[data-cursor]');
      ring.current?.classList.toggle('cursor-ring--active', !!interactive);
    };

    window.addEventListener('pointermove', move);
    window.addEventListener('pointerover', over);
    return () => {
      window.removeEventListener('pointermove', move);
      window.removeEventListener('pointerover', over);
    };
  }, [fine, reduced]);

  if (!fine || reduced) return null;

  return (
    <>
      <div ref={ring} className="cursor-ring" aria-hidden="true" />
      <div ref={dot} className="cursor-dot" aria-hidden="true" />
    </>
  );
}
