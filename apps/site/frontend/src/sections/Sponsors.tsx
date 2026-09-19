import Reveal from '../components/Reveal';
import { SPONSORS } from '../data/sponsors';

/**
 * The marquee runs two identical halves so the loop has no seam. Each half
 * repeats the list so it stays wider than the widest screen; a shorter half
 * leaves a blank stretch at the end of every loop.
 */
const MARQUEE_HALF = Array.from({ length: 4 }, () => SPONSORS).flat();
const MARQUEE_ROW = [...MARQUEE_HALF, ...MARQUEE_HALF];

export default function Sponsors() {
  return (
    <section className="sponsors" aria-label="Partners and sponsors">
      <div className="container">
        <Reveal>
          <span className="mono-label">Backed by</span>
        </Reveal>
      </div>
      <div className="sponsors__marquee" aria-hidden="true">
        <div className="sponsors__track">
          {MARQUEE_ROW.map((s, i) => (
            <span key={i} className="sponsors__item">
              {s.logo ? (
                <img className="sponsors__logo" src={s.logo.src} width={s.logo.width} height={s.logo.height} alt="" />
              ) : (
                s.name
              )}
            </span>
          ))}
        </div>
      </div>
    </section>
  );
}
