import { ArrowRight, CalendarDays, MessageCircle, Users } from 'lucide-react';
import Join from '../sections/Join';
import SplitText from '../components/SplitText';
import Reveal from '../components/Reveal';
import MagneticButton from '../components/MagneticButton';
import { CONTACT } from '../data/projects';
import PageMeta from '../components/PageMeta';

const PERKS = [
  { t: 'Hands-on hardware', b: 'Build and test working mechatronic systems.' },
  { t: 'Mentorship', b: 'Learn from experienced members across mechanical, electrical, and software.' },
  { t: 'Competitions', b: 'Represent Waterloo at events such as ACE 2026.' },
  { t: 'Community impact', b: 'Build e-NABLE devices for people who need them.' },
];

export default function JoinPage() {
  return (
    <main id="main" className="joinpage">
      <PageMeta
        title="How to Join Biotron | UW Design Team"
        description="Check our calendar, come to a meeting, and start building biomechatronic systems with Biotron."
      />
      <section className="container joinpage__head">
        <span className="eyebrow mono-label">How to join</span>
        <SplitText
          as="h1"
          className="joinpage__title"
          text="Start by showing up."
          by="word"
        />
        <p className="joinpage__lead">
          There is no application. Students from any faculty and experience level can come to a
          meeting, meet the team, and find something worth building.
        </p>
        <div className="joinpage__actions">
          <MagneticButton to="/calendar" variant="primary" magnetic={false}>
            Check the calendar <ArrowRight size={18} />
          </MagneticButton>
          <MagneticButton href={CONTACT.discord} variant="ghost" magnetic={false}>
            Join Discord <ArrowRight size={18} />
          </MagneticButton>
        </div>
      </section>

      <section className="container joinpage__steps" aria-label="How to join Biotron">
        <Reveal className="joinpage__step glass">
          <div className="joinpage__stephead">
            <span className="joinpage__stepnum tabular">01</span>
            <CalendarDays size={22} aria-hidden="true" />
          </div>
          <h2>Find a meeting</h2>
          <p>
            Check the calendar for the next open meeting. Discord carries the latest time and room
            if plans change.
          </p>
          <MagneticButton to="/calendar" variant="ghost" magnetic={false}>
            Open calendar <ArrowRight size={16} />
          </MagneticButton>
        </Reveal>
        <Reveal delay={0.05} className="joinpage__step glass">
          <div className="joinpage__stephead">
            <span className="joinpage__stepnum tabular">02</span>
            <Users size={22} aria-hidden="true" />
          </div>
          <h2>Come say hello</h2>
          <p>
            Show up, meet the leads, and sit with a sub-team. You do not need experience or a
            polished project idea to start.
          </p>
        </Reveal>
        <Reveal delay={0.1} className="joinpage__step glass">
          <div className="joinpage__stephead">
            <span className="joinpage__stepnum tabular">03</span>
            <MessageCircle size={22} aria-hidden="true" />
          </div>
          <h2>Get connected</h2>
          <p>
            Join Discord, choose your sub-team roles, and open the team Notion from the welcome
            channel. That is where work, docs, and updates live.
          </p>
          <MagneticButton href={CONTACT.discord} variant="ghost" magnetic={false}>
            Join Discord <ArrowRight size={16} />
          </MagneticButton>
        </Reveal>
      </section>

      <section className="container joinpage__perks">
        {PERKS.map((p, i) => (
          <Reveal key={p.t} delay={i * 0.05} className="joinpage__perk glass">
            <span className="joinpage__perknum tabular">0{i + 1}</span>
            <h2 className="joinpage__perktitle">{p.t}</h2>
            <p className="joinpage__perkbody">{p.b}</p>
          </Reveal>
        ))}
      </section>

      <Join showGuideLink={false} />
    </main>
  );
}
