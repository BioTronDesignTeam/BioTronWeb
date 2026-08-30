import { Link } from 'react-router-dom';
import { ArrowUpRight } from 'lucide-react';
import { PAST_PROJECTS } from '../data/projects';
import Reveal from '../components/Reveal';
import SplitText from '../components/SplitText';

export default function PastProjects() {
  return (
    <main id="main" className="past">
      <section className="container past__head">
        <span className="eyebrow mono-label">Archive</span>
        <SplitText as="h1" className="past__title" text="Where we’ve been." by="word" />
        <p className="past__lead">
          A selection of the prototypes and devices that shaped our team — and seeded today’s work.
        </p>
      </section>

      <section className="container past__grid">
        {PAST_PROJECTS.map((p, i) => (
          <Reveal key={p.name} delay={i * 0.05}>
            <article className="past__card glass">
              <div className="past__cardtop">
                <h2 className="past__name">{p.name}</h2>
                {p.partner && <span className="past__partner">with {p.partner}</span>}
              </div>
              <p className="past__blurb">{p.blurb}</p>
              <div className="past__tags">
                {p.tags.map((t) => (
                  <span key={t} className="past__tag">
                    {t}
                  </span>
                ))}
              </div>
            </article>
          </Reveal>
        ))}
      </section>

      <section className="container past__next">
        <div className="past__nextcard glass">
          <div>
            <span className="eyebrow mono-label">What’s next</span>
            <h2>See what those lessons are powering now.</h2>
            <p>Explore today’s projects, or join the team building the next one.</p>
          </div>
          <div className="past__nextactions">
            <Link to="/projects" className="past__nextprimary">
              Active projects <ArrowUpRight size={17} />
            </Link>
            <Link to="/join" className="past__nextsecondary">
              Join the team
            </Link>
          </div>
        </div>
      </section>
    </main>
  );
}
