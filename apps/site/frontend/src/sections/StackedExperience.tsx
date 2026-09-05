import { ArrowUpRight } from 'lucide-react';
import { Link } from 'react-router-dom';
import Reveal from '../components/Reveal';
import SplitText from '../components/SplitText';
import CountUp from '../components/CountUp';
import ModelFallback from '../components/ModelFallback';
import ActionButton from '../components/ActionButton';
import { PROJECTS, TEAM_STATS } from '../data/projects';

/**
 * Conventional stacked layout shown when 3D is unavailable (no WebGL) or under
 * prefers-reduced-motion. Same content as the fly-through, no camera travel.
 */
export default function StackedExperience() {
  return (
    <>
      {/* Hero */}
      <section className="shero">
        <div className="container shero__inner">
          <span className="eyebrow mono-label">UW Biomechatronics Design Team</span>
          <SplitText
            as="h1"
            className="shero__title"
            text="We build technology that moves with people."
            by="word"
          />
          <p className="shero__lead">
            Our projects include powered exoskeletons, EMG wearables, and 3D-printed assistive
            devices.
          </p>
          <div className="shero__cta">
            <ActionButton to="/projects" variant="primary">
              Explore projects <ArrowUpRight size={18} />
            </ActionButton>
            <ActionButton to="/join" variant="ghost">
              Join us
            </ActionButton>
          </div>
        </div>
      </section>

      {/* About */}
      <section className="sabout container" id="about">
        <Reveal>
          <span className="eyebrow mono-label">About</span>
          <h2 className="sabout__title">We build machines that move people.</h2>
        </Reveal>
        <Reveal delay={0.1}>
          <p className="sabout__body">
            We are a University of Waterloo student team that solves biomedical problems by
            building and testing real hardware.
          </p>
        </Reveal>
        <div className="sabout__stats">
          {TEAM_STATS.map((s) => (
            <Reveal key={s.label} className="sabout__stat">
              <span className="sabout__num">
                <CountUp value={s.value} suffix={s.suffix} plain={'plain' in s && s.plain} />
              </span>
              <span className="sabout__lbl">{s.label}</span>
            </Reveal>
          ))}
        </div>
      </section>

      {/* Projects */}
      <section className="sprojects container" id="projects">
        <Reveal>
          <span className="eyebrow mono-label">Projects</span>
          <h2 className="sprojects__title">Three problems we’re solving now.</h2>
        </Reveal>
        <div className="sprojects__list">
          {PROJECTS.map((p, i) => (
            <Reveal key={p.slug} delay={i * 0.05}>
              <article className="sproject glass" style={{ ['--card-accent' as string]: p.accentHex }}>
                <div className="sproject__visual">
                  <ModelFallback project={p} />
                </div>
                <div className="sproject__content">
                  <span className="mono-label">{p.status}</span>
                  <h3 className="sproject__name">{p.name}</h3>
                  <p className="sproject__tag">
                    {p.tagline}
                  </p>
                  <p className="sproject__blurb">{p.blurb}</p>
                  <Link to={`/projects/${p.slug}`} className="sproject__cta">
                    View project <ArrowUpRight size={16} />
                  </Link>
                </div>
              </article>
            </Reveal>
          ))}
        </div>
      </section>
    </>
  );
}
