import { useEffect } from 'react';
import { useLocation } from 'react-router-dom';
import Flythrough from '../sections/Flythrough';
import StackedExperience from '../sections/StackedExperience';
import Sponsors from '../sections/Sponsors';
import Join from '../sections/Join';
import {
  useReducedMotion,
  useWebGLSupported,
  useIsMobile,
  useIsLandscapePhone,
} from '../lib/hooks';
import { scrollToElement } from '../lib/SmoothScroll';
import type { Stop } from '../lib/store';
import PageMeta from '../components/PageMeta';

export default function Home() {
  const reduced = useReducedMotion();
  const webgl = useWebGLSupported();
  const mobile = useIsMobile(640);
  const landscapePhone = useIsLandscapePhone();
  const location = useLocation();

  // Immersive fly-through needs WebGL + motion. Phones get the lighter stacked
  // layout for reliability and battery, but tablets/desktop keep the room.
  const immersive = webgl && !reduced && !mobile && !landscapePhone;

  useEffect(() => {
    const stop = (location.state as { stop?: Stop } | null)?.stop;
    if (immersive || !stop) return;

    const target = stop === 'about' ? '#about' : '#projects';
    const frame = window.requestAnimationFrame(() => scrollToElement(target, -72));
    return () => window.cancelAnimationFrame(frame);
  }, [immersive, location.key, location.state]);

  return (
    <main id="main">
      <PageMeta
        title="Biotron | UW Biomechatronics Design Team"
        description="We are a University of Waterloo student team building exoskeletons, EMG wearables, and assistive devices."
      />
      {immersive ? <Flythrough /> : <StackedExperience />}
      <div className="post-room">
        <Sponsors />
        <Join />
      </div>
    </main>
  );
}
