import { ArrowRight } from 'lucide-react';
import Reveal from '../components/Reveal';
import SplitText from '../components/SplitText';
import ActionButton from '../components/ActionButton';
import { SUBTEAMS } from '../data/projects';

interface JoinProps {
  showGuideLink?: boolean;
}

export default function Join({ showGuideLink = true }: JoinProps) {
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
            text="Build biomechatronics with us."
            by="word"
            trigger
          />
          <Reveal delay={0.1}>
            <p className="join__lead">
              We welcome every discipline and experience level. Bring curiosity, commitment, and
              a willingness to learn. Choose a sub-team:
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

        {showGuideLink && (
          <Reveal className="join__cta">
            <div className="join__guide">
              <ActionButton to="/join" variant="primary">
                See how to join <ArrowRight size={18} />
              </ActionButton>
              <span>No application. Find a meeting, show up, and start building with us.</span>
            </div>
          </Reveal>
        )}
      </div>
    </section>
  );
}
