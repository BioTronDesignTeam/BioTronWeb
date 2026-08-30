import Flythrough from '../sections/Flythrough';
import StackedExperience from '../sections/StackedExperience';
import Sponsors from '../sections/Sponsors';
import Join from '../sections/Join';
import { useReducedMotion, useWebGLSupported, useIsMobile } from '../lib/hooks';

export default function Home() {
  const reduced = useReducedMotion();
  const webgl = useWebGLSupported();
  const mobile = useIsMobile(640);

  // Immersive fly-through needs WebGL + motion. Phones get the lighter stacked
  // layout for reliability and battery, but tablets/desktop keep the room.
  const immersive = webgl && !reduced && !mobile;

  return (
    <main id="main">
      {immersive ? <Flythrough /> : <StackedExperience />}
      <div className="post-room">
        <Sponsors />
        <Join />
      </div>
    </main>
  );
}
