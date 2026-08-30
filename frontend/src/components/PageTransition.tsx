import { useRef, type ReactNode } from 'react';
import { useLocation } from 'react-router-dom';
import { gsap, useGSAP } from '../lib/gsap';
import { useReducedMotion } from '../lib/hooks';

/** Fades/lifts routed page content in on each navigation. */
export default function PageTransition({ children }: { children: ReactNode }) {
  const ref = useRef<HTMLDivElement>(null);
  const { pathname } = useLocation();
  const reduced = useReducedMotion();

  useGSAP(
    () => {
      if (reduced || !ref.current) return;
      // Opacity-only: a transform here would become the containing block for the
      // fixed 3D canvas inside this subtree and break its full-screen sizing.
      gsap.fromTo(
        ref.current,
        { opacity: 0 },
        { opacity: 1, duration: 0.5, ease: 'power2.out' },
      );
    },
    { dependencies: [pathname, reduced] },
  );

  return <div ref={ref}>{children}</div>;
}
