import { ArrowUpRight } from 'lucide-react';
import Join from '../sections/Join';
import SplitText from '../components/SplitText';
import Reveal from '../components/Reveal';
import MagneticButton from '../components/MagneticButton';
import { JOIN_EMAIL_HREF } from '../data/projects';
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
        title="Join Biotron | UW Design Team"
        description="Join our University of Waterloo team to design, build, and test biomechatronic systems."
      />
      <section className="container joinpage__head">
        <span className="eyebrow mono-label">Recruitment open</span>
        <SplitText
          as="h1"
          className="joinpage__title"
          text="Find your place on the team."
          by="word"
        />
        <p className="joinpage__lead">
          Students from any faculty can join. Design in CAD, build circuits, write software, or
          grow our community.
        </p>
        <div className="joinpage__apply">
          <MagneticButton href={JOIN_EMAIL_HREF} variant="primary">
            Apply by email <ArrowUpRight size={18} />
          </MagneticButton>
          <p>
            Tell us your program, year, interests, and what you hope to build or learn. We will
            connect you with the right sub-team.
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
