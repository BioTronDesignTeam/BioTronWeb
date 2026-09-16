import { ArrowRight, CalendarDays, ClipboardCheck, MessageCircle } from 'lucide-react';
import Join from '../sections/Join';
import SplitText from '../components/SplitText';
import Reveal from '../components/Reveal';
import ActionButton from '../components/ActionButton';
import { CONTACT } from '../data/projects';
import PageMeta from '../components/PageMeta';

const PERKS = [
  { t: 'Hands-on hardware', b: 'Build and test working mechatronic systems.' },
  { t: 'Mentorship', b: 'Learn from experienced members across mechanical, electrical, and software.' },
  { t: 'Competitions', b: 'Represent Waterloo at events such as ACE 2027.' },
  { t: 'Community impact', b: 'Build e-NABLE devices for people who need them.' },
];

export default function JoinPage() {
  return (
    <main id="main" className="joinpage">
      <PageMeta
        title="Join | BioTron"
        description="Come to any Biotron meeting, join our Discord and Notion, and finish your sub-team onboarding."
      />
      <section className="container joinpage__head">
        <span className="eyebrow mono-label">How to join</span>
        <SplitText
          as="h1"
          className="joinpage__title"
          text="Start by showing up."
          by="word"
        />
      </section>

      <section className="container joinpage__steps" aria-label="How to join Biotron">
        <Reveal className="joinpage__step glass">
          <div className="joinpage__stephead">
            <span className="joinpage__stepnum tabular">01</span>
            <CalendarDays size={22} aria-hidden="true" />
          </div>
          <h2>Come to any meeting</h2>
          <p>
            Every meeting on the calendar is open to you. Drop in on whichever one you like, meet
            the leads, and sit with a sub-team. You need no experience to start.
          </p>
          <ActionButton to="/calendar" variant="ghost">
            Open calendar <ArrowRight size={16} />
          </ActionButton>
        </Reveal>
        <Reveal delay={0.05} className="joinpage__step glass">
          <div className="joinpage__stephead">
            <span className="joinpage__stepnum tabular">02</span>
            <MessageCircle size={22} aria-hidden="true" />
          </div>
          <h2>Join Discord and Notion</h2>
          <p>
            Discord carries the day-to-day conversation and the latest meeting times. Notion holds
            the work, the documents, and the onboarding material for every sub-team.
          </p>
          <div className="joinpage__steplinks">
            <ActionButton href={CONTACT.discord} variant="ghost">
              Join Discord <ArrowRight size={16} />
            </ActionButton>
            {CONTACT.notion && (
              <ActionButton href={CONTACT.notion} variant="ghost">
                Open Notion <ArrowRight size={16} />
              </ActionButton>
            )}
          </div>
        </Reveal>
        <Reveal delay={0.1} className="joinpage__step glass">
          <div className="joinpage__stephead">
            <span className="joinpage__stepnum tabular">03</span>
            <ClipboardCheck size={22} aria-hidden="true" />
          </div>
          <h2>Finish your onboarding</h2>
          <p>
            Each project and sub-team sets its own onboarding, so what you complete depends on
            where you land. Your lead will point you at the right one in Notion. Finish it and you
            are ready to pick up work.
          </p>
        </Reveal>
      </section>

      <section className="container joinpage__perks">
        {PERKS.map((p, i) => (
          <Reveal key={p.t} delay={i * 0.05} className="joinpage__perk glass">
            <h2 className="joinpage__perktitle">{p.t}</h2>
            <p className="joinpage__perkbody">{p.b}</p>
          </Reveal>
        ))}
      </section>

      <Join showGuideLink={false} />
    </main>
  );
}
