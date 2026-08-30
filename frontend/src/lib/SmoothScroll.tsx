import { useEffect } from 'react';
import { useLocation } from 'react-router-dom';
import Lenis from 'lenis';
import { gsap, ScrollTrigger } from './gsap';
import { useReducedMotion } from './hooks';

let lenisInstance: Lenis | null = null;

/** Programmatic scroll used by nav links and click-to-jump hotspots. */
export function scrollToY(y: number, opts?: { duration?: number; immediate?: boolean }) {
  if (lenisInstance) {
    lenisInstance.scrollTo(y, { duration: opts?.duration ?? 1.2, immediate: opts?.immediate });
  } else {
    window.scrollTo({ top: y, behavior: opts?.immediate ? 'auto' : 'smooth' });
  }
}

export function scrollToElement(target: string | HTMLElement, offset = 0) {
  if (lenisInstance) {
    lenisInstance.scrollTo(target, { offset, duration: 1.2 });
  } else {
    const el = typeof target === 'string' ? document.querySelector(target) : target;
    if (el) window.scrollTo({ top: (el as HTMLElement).offsetTop + offset, behavior: 'smooth' });
  }
}

/**
 * Wires Lenis smooth scrolling to GSAP's ticker + ScrollTrigger (the canonical
 * integration). Disabled entirely under prefers-reduced-motion so the page uses
 * plain native scrolling.
 */
export default function SmoothScroll({ children }: { children: React.ReactNode }) {
  const reduced = useReducedMotion();
  const { pathname, hash } = useLocation();

  useEffect(() => {
    if (reduced) return;

    const lenis = new Lenis({
      duration: 1.1,
      easing: (t) => Math.min(1, 1.001 - Math.pow(2, -10 * t)),
      smoothWheel: true,
    });
    lenisInstance = lenis;

    lenis.on('scroll', ScrollTrigger.update);

    const tick = (time: number) => lenis.raf(time * 1000);
    gsap.ticker.add(tick);
    gsap.ticker.lagSmoothing(0);

    return () => {
      gsap.ticker.remove(tick);
      lenis.destroy();
      lenisInstance = null;
    };
  }, [reduced]);

  // Reset on route changes, or honor an in-page destination when provided.
  useEffect(() => {
    const frame = window.requestAnimationFrame(() => {
      if (hash && document.querySelector(hash)) scrollToElement(hash, -96);
      else scrollToY(0, { immediate: true });
      ScrollTrigger.refresh();
    });

    return () => window.cancelAnimationFrame(frame);
  }, [hash, pathname]);

  return <>{children}</>;
}
