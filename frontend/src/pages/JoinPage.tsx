import { ArrowUpRight } from 'lucide-react';
import Join from '../sections/Join';
import SplitText from '../components/SplitText';
import Reveal from '../components/Reveal';
import MagneticButton from '../components/MagneticButton';
import { JOIN_EMAIL_HREF } from '../data/projects';

const PERKS = [
  { t: 'Hands-on hardware', b: 'Design, build, and test real mechatronic systems — not just slides.' },
  { t: 'Mentorship', b: 'Learn from senior members across mechanical, electrical, and software.' },
  { t: 'Competitions', b: 'Represent Waterloo on international stages like ACE 2026.' },
  { t: 'Community impact', b: 'Ship assistive devices that change lives through e-NABLE.' },
];

export default function JoinPage() {
  return (
    <main id="main" className="joinpage">
      <section className="container joinpage__head">
        <span className="eyebrow mono-label">Recruitment open</span>
        <SplitText
          as="h1"
          className="joinpage__title"
          text="Find your place on the team."
          by="word"
        />
        <p className="joinpage__lead">
          We welcome all faculties and skill levels. Whether you live in CAD, solder boards, train
          models, or rally a community — there’s a seat for you.
        </p>
        <div className="joinpage__apply">
          <MagneticButton href={JOIN_EMAIL_HREF} variant="primary">
            Start your application <ArrowUpRight size={18} />
          </MagneticButton>
          <p>
            The email template asks for your name, program and year, interests, and what you want
            to build or learn so we can route you to the right sub-team.
          </p>
        </div>
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

      <Join />
    </main>
  );
}
