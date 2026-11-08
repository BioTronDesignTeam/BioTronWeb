import { useEffect, useRef } from 'react';
import { gsap, ScrollTrigger } from '../lib/gsap';

/** Thin top progress bar tracking page scroll. */
export default function ScrollProgress() {
  const bar = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const st = ScrollTrigger.create({
      start: 0,
      end: 'max',
      onUpdate: (self) => {
        if (bar.current) gsap.set(bar.current, { scaleX: self.progress });
      },
    });
    return () => st.kill();
  }, []);

  return <div ref={bar} className="scroll-progress" aria-hidden="true" />;
}
