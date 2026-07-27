import Reveal from '../components/Reveal';

// Placeholder partner names — swap for real sponsor logos when provided.
const SPONSORS = [
  'University of Waterloo',
  'Engineering Society',
  'WEEF',
  'Sedra Student Design Centre',
];

export default function Sponsors() {
  const row = [...SPONSORS, ...SPONSORS];
  return (
    <section className="sponsors" aria-label="Partners and sponsors">
      <div className="container">
        <Reveal>
          <span className="mono-label">Backed by</span>
        </Reveal>
      </div>
      <div className="sponsors__marquee" aria-hidden="true">
        <div className="sponsors__track">
          {row.map((s, i) => (
            <span key={i} className="sponsors__item">
              {s}
            </span>
          ))}
        </div>
      </div>
    </section>
  );
}
