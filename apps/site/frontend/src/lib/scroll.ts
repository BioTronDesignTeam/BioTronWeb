import type Lenis from 'lenis';

/**
 * Registry for the single Lenis instance owned by SmoothScroll. Kept out of the
 * component file so Fast Refresh can preserve component state.
 */
let lenisInstance: Lenis | null = null;

/** Called by SmoothScroll as it mounts and unmounts. */
export function setLenis(instance: Lenis | null) {
  lenisInstance = instance;
}

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
