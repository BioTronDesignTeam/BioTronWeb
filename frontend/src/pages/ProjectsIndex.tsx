import { Link } from 'react-router-dom';
import { ArrowUpRight } from 'lucide-react';
import { PAST_PROJECTS, PROJECTS } from '../data/projects';
import Reveal from '../components/Reveal';
import SplitText from '../components/SplitText';
import ModelFallback from '../components/ModelFallback';
import PageMeta from '../components/PageMeta';

export default function ProjectsIndex() {
  return (
    <main id="main" className="pindex">
      <PageMeta
        title="Projects — Biotron"
        description="Explore Biotron’s current biomechatronics work and the projects that shaped it."
      />
      <section className="container pindex__head" id="current">
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
                <p className="pindex__tag">
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

      <section className="pindex__archive" id="past">
        <div className="container past__head">
          <span className="eyebrow mono-label">Past projects</span>
          <SplitText as="h2" className="past__title" text="Where we’ve been." by="word" />
          <p className="past__lead">
            A selection of the prototypes and devices that shaped our team — and seeded today’s work.
          </p>
        </div>

        <div className="container past__grid">
          {PAST_PROJECTS.map((p, i) => (
            <Reveal key={p.name} delay={i * 0.05}>
              <article className="past__card glass">
                <div className="past__cardtop">
                  <h3 className="past__name">{p.name}</h3>
                  {p.partner && <span className="past__partner">with {p.partner}</span>}
                </div>
                <p className="past__blurb">{p.blurb}</p>
                <div className="past__tags">
                  {p.tags.map((tag) => (
                    <span key={tag} className="past__tag">
                      {tag}
                    </span>
                  ))}
                </div>
              </article>
            </Reveal>
          ))}
        </div>
      </section>
    </main>
  );
}
