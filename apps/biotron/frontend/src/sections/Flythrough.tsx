import { Suspense, lazy, useEffect, useRef } from 'react';
import { useLocation } from 'react-router-dom';
import { ChevronDown, ArrowUpRight } from 'lucide-react';
import { gsap, ScrollTrigger } from '../lib/gsap';
import { useScene, type Stop } from '../lib/store';
import { registerFlythroughRange, clearFlythroughRange, jumpToStop } from '../lib/flythrough';
import ActionButton from '../components/ActionButton';
import RoomPopup from '../components/RoomPopup';

const Scene = lazy(() => import('../three/Scene'));

/**
 * The immersive Home centerpiece: a tall scroll section whose progress drives
 * the camera around the workshop room (via the scene store). A hero overlay
 * fades out as the tour begins; RoomPopup shows info at each dwell.
 */
export default function Flythrough() {
  const driver = useRef<HTMLDivElement>(null);
  const hero = useRef<HTMLDivElement>(null);
  const ready = useScene((s) => s.ready);
  const setProgress = useScene((s) => s.setProgress);
  const location = useLocation();

  useEffect(() => {
    const el = driver.current;
    if (!el) return;

    const st = ScrollTrigger.create({
      trigger: el,
      start: 'top top',
      end: 'bottom bottom',
      scrub: true,
      onUpdate: (self) => {
        setProgress(self.progress);
        // Fade the hero overlay out across the first ~12% of the tour.
        if (hero.current) {
          const o = gsap.utils.clamp(0, 1, 1 - self.progress / 0.1);
          gsap.set(hero.current, { autoAlpha: o, y: -self.progress * 60 });
        }
      },
    });

    ScrollTrigger.refresh();
    registerFlythroughRange(st.start, st.end);

    return () => {
      st.kill();
      clearFlythroughRange();
    };
  }, [setProgress]);

  // Honor navigation requests to a specific stop (e.g. nav "About" from another page).
  useEffect(() => {
    const stop = (location.state as { stop?: Stop } | null)?.stop;
    if (stop) {
      const id = window.setTimeout(() => jumpToStop(stop), 400);
      return () => window.clearTimeout(id);
    }
  }, [location.state]);

  return (
    <section className="flythrough" ref={driver} aria-label="Interactive workshop tour">
      {/* Sticky stage holding overlays above the fixed 3D canvas */}
      <div className="flythrough__stage">
        <Suspense fallback={<div className="flythrough__loading" />}>
          <Scene />
        </Suspense>

        {/* Hero overlay */}
        <div className="flythrough__hero" ref={hero}>
          <div className="flythrough__hero-content">
            <span className="eyebrow mono-label">UW Biomechatronics Design Team</span>
            <h1 className="flythrough__title">
              <span className="line">Welcome to</span>
              <span className="line accent">Biotron.</span>
            </h1>
            <p className="flythrough__lead">
              Scroll through our workshop.
            </p>
            <div className="flythrough__cta">
              <ActionButton onClick={() => jumpToStop('exo')} variant="primary">
                Explore projects <ArrowUpRight size={18} />
              </ActionButton>
              <ActionButton to="/join" variant="ghost">
                Join us
              </ActionButton>
            </div>
            <div className={`flythrough__hint ${ready ? 'is-ready' : ''}`}>
              <ChevronDown size={18} />
              <span>Scroll to begin</span>
            </div>
          </div>
        </div>

        {/* Stop info popups */}
        <RoomPopup />

        {!ready && (
          <div className="flythrough__boot" role="status" aria-live="polite">
            <div className="flythrough__bootbar">
              <span />
            </div>
            <span className="mono-label">Loading workshop…</span>
          </div>
        )}
      </div>
    </section>
  );
}
