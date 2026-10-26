import { ArrowUpRight } from 'lucide-react';
import PageMeta from '../components/PageMeta';
import Reveal from '../components/Reveal';
import SplitText from '../components/SplitText';
import { CONTACT } from '../data/projects';
import { SPONSORS, SPONSORSHIP_AREAS } from '../data/sponsors';

const sponsorshipEmail = `mailto:${CONTACT.email}?subject=${encodeURIComponent('Biotron sponsorship')}`;

export default function SponsorsPage() {
  return (
    <main id="main" className="sponsorspage">
      <PageMeta path="/sponsors" />

      <section className="container sponsorspage__hero">
        <span className="eyebrow mono-label">Partner with Biotron</span>
        <SplitText
          as="h1"
          className="sponsorspage__title"
          text="Help power our projects."
          by="word"
        />
        <p className="sponsorspage__lead">
          Partners fund the tools, materials, and shop time our students need. Their support turns
          student ideas into working hardware and assistive devices people can use.
        </p>
        <a className="sponsorspage__primary" href={sponsorshipEmail}>
          Become a sponsor <ArrowUpRight size={18} />
        </a>
      </section>

      <section className="container sponsorspage__supporters" aria-labelledby="supporters-title">
        <Reveal>
          <span className="eyebrow mono-label">Backed by</span>
          <h2 id="supporters-title">Our current sponsors.</h2>
        </Reveal>
        <div className="sponsorspage__supportergrid">
          {SPONSORS.map((sponsor, index) => (
            <Reveal key={sponsor.name} delay={index * 0.05}>
              <div className="sponsorspage__supporter">
                <img
                  className={`sponsorspage__logo sponsorspage__logo--${sponsor.logo.kind}`}
                  src={sponsor.logo.src}
                  width={sponsor.logo.width}
                  height={sponsor.logo.height}
                  alt={sponsor.logo.kind === 'wordmark' ? sponsor.name : ''}
                />
                {sponsor.logo.kind === 'mark' && <span>{sponsor.name}</span>}
              </div>
            </Reveal>
          ))}
        </div>
      </section>

      <section className="container sponsorspage__areas" aria-labelledby="partnership-title">
        <Reveal>
          <span className="eyebrow mono-label">What your support enables</span>
          <h2 id="partnership-title">Your support reaches the workbench.</h2>
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
              <h2>Build a partnership that fits.</h2>
              <p>Tell us what your organization wants to support. We will shape a partnership around it.</p>
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
