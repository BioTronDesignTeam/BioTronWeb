import Reveal from '../components/Reveal';
import { SPONSORS } from '../data/sponsors';

/** The marquee runs two copies of the list so the loop has no seam. */
const MARQUEE_ROW = [...SPONSORS, ...SPONSORS];

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
              {s}
            </span>
          ))}
        </div>
      </div>
    </section>
  );
}
