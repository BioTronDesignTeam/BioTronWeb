import { Link } from 'react-router-dom';
import { ArrowUpRight } from 'lucide-react';
import { PROJECTS } from '../data/projects';
import Reveal from '../components/Reveal';
import SplitText from '../components/SplitText';
import ModelFallback from '../components/ModelFallback';

export default function ProjectsIndex() {
  return (
    <main id="main" className="pindex">
      <section className="container pindex__head">
        <span className="eyebrow mono-label">Active projects</span>
        <SplitText
          as="h1"
          className="pindex__title"
          text="Three problems we’re solving now."
          by="word"
        />
        <p className="pindex__lead">
          Each project is a year-round, multi-disciplinary engineering effort. Explore the work —
          and the 3D models behind it.
        </p>
      </section>

      <section className="container pindex__list">
        {PROJECTS.map((p, i) => (
          <Reveal key={p.slug} delay={i * 0.05}>
            <Link
              to={`/projects/${p.slug}`}
              className="pindex__row glass"
              style={{ ['--card-accent' as string]: p.accentHex }}
            >
              <div className="pindex__visual">
                <ModelFallback project={p} />
              </div>
              <div className="pindex__info">
                <span className="mono-label">{p.status}</span>
                <h2 className="pindex__name">{p.name}</h2>
                <p className="pindex__tag" style={{ color: p.accentHex }}>
                  {p.tagline}
                </p>
                <p className="pindex__blurb">{p.blurb}</p>
              </div>
              <span className="pindex__cta">
                View project <ArrowUpRight size={18} />
              </span>
            </Link>
          </Reveal>
        ))}
      </section>
    </main>
  );
}
