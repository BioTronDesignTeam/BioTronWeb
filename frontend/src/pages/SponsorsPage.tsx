import { ArrowUpRight } from 'lucide-react';
import PageMeta from '../components/PageMeta';
import Reveal from '../components/Reveal';
import SplitText from '../components/SplitText';
import { CONTACT } from '../data/projects';
import { SPONSORS, SPONSORSHIP_AREAS } from '../data/sponsors';

const sponsorshipEmail = `mailto:${CONTACT.email}?subject=${encodeURIComponent('BioTron sponsorship')}`;

export default function SponsorsPage() {
  return (
    <main id="main" className="sponsorspage">
      <PageMeta
        title="Sponsors — Biotron"
        description="Meet the organizations supporting BioTron and learn how to partner with the University of Waterloo biomechatronics design team."
      />

      <section className="container sponsorspage__hero">
        <span className="eyebrow mono-label">Partner with BioTron</span>
        <SplitText
          as="h1"
          className="sponsorspage__title"
          text="Help us build what comes next."
          by="word"
        />
        <p className="sponsorspage__lead">
          Our partners give students the tools and opportunities to engineer practical biomedical
          systems. In return, they become part of a team turning ambitious ideas into working
          hardware.
        </p>
        <a className="sponsorspage__primary" href={sponsorshipEmail}>
          Start a conversation <ArrowUpRight size={18} />
        </a>
      </section>

      <section className="container sponsorspage__supporters" aria-labelledby="supporters-title">
        <Reveal>
          <span className="eyebrow mono-label">Backed by</span>
          <h2 id="supporters-title">Our current supporters.</h2>
        </Reveal>
        <div className="sponsorspage__supportergrid">
          {SPONSORS.map((sponsor, index) => (
            <Reveal key={sponsor} delay={index * 0.05}>
              <div className="sponsorspage__supporter glass">{sponsor}</div>
            </Reveal>
          ))}
        </div>
      </section>

      <section className="container sponsorspage__areas" aria-labelledby="partnership-title">
        <Reveal>
          <span className="eyebrow mono-label">What partnership enables</span>
          <h2 id="partnership-title">Support that reaches the workbench.</h2>
        </Reveal>
        <div className="sponsorspage__areagrid">
          {SPONSORSHIP_AREAS.map((area, index) => (
            <Reveal key={area.title} delay={index * 0.05}>
              <article className="sponsorspage__area glass">
                <span className="mono-label">0{index + 1}</span>
                <h3>{area.title}</h3>
                <p>{area.body}</p>
              </article>
            </Reveal>
          ))}
        </div>
      </section>

      <section className="container sponsorspage__contact">
        <Reveal>
          <div className="sponsorspage__contactcard glass">
            <div>
              <span className="eyebrow mono-label">Work with us</span>
              <h2>Let’s find the right way to partner.</h2>
              <p>Tell us what your organization wants to support and we’ll take it from there.</p>
            </div>
            <a className="sponsorspage__primary" href={sponsorshipEmail}>
              Contact the team <ArrowUpRight size={18} />
            </a>
          </div>
        </Reveal>
      </section>
    </main>
  );
}
