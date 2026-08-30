import { ArrowUpRight } from 'lucide-react';
import Reveal from '../components/Reveal';
import SplitText from '../components/SplitText';
import MagneticButton from '../components/MagneticButton';
import { SUBTEAMS, CONTACT, JOIN_EMAIL_HREF } from '../data/projects';

export default function Join() {
  return (
    <section className="join" id="join">
      <div className="container join__inner">
        <div className="join__head">
          <Reveal>
            <span className="eyebrow mono-label">Join the team</span>
          </Reveal>
          <SplitText
            as="h2"
            className="join__title"
            text="Build the future of biomechatronics with us."
            by="word"
            trigger
          />
          <Reveal delay={0.1}>
            <p className="join__lead">
              We recruit engineers, designers, and scientists across every discipline. No
              experience required, just curiosity and commitment. Find your sub-team:
            </p>
          </Reveal>
        </div>

        <div className="join__grid">
          {SUBTEAMS.map((t, i) => (
            <Reveal key={t.name} delay={i * 0.05} className="join__card glass">
              <span className="join__num tabular">0{i + 1}</span>
              <h3 className="join__cardtitle">{t.name}</h3>
              <p className="join__cardbody">{t.blurb}</p>
            </Reveal>
          ))}
        </div>

        <Reveal className="join__cta">
          <div className="join__apply">
            <MagneticButton href={JOIN_EMAIL_HREF} variant="primary">
              Apply by email <ArrowUpRight size={18} />
            </MagneticButton>
            <span>Include your program, year, interests, and what you want to build or learn.</span>
          </div>
          <a href={CONTACT.facebook} target="_blank" rel="noreferrer" className="join__alt">
            or follow us on Facebook
          </a>
        </Reveal>
      </div>
    </section>
  );
}
