import { Link } from 'react-router-dom';
import { Mail, Facebook, ArrowUp } from 'lucide-react';
import { CONTACT, PROJECTS } from '../data/projects';
import { scrollToY } from '../lib/SmoothScroll';

export default function Footer() {
  return (
    <footer className="footer" id="contact">
      <div className="container-wide footer__inner">
        <div className="footer__brand">
          <span className="footer__word">BIOTRON</span>
          <p className="footer__tag">Engineering mechatronic solutions to biomedical challenges.</p>
          <button className="footer__top" onClick={() => scrollToY(0)} aria-label="Back to top">
            <ArrowUp size={16} /> Back to top
          </button>
        </div>

        <nav className="footer__col" aria-label="Projects">
          <h3 className="footer__heading">Projects</h3>
          {PROJECTS.map((p) => (
            <Link key={p.slug} to={`/projects/${p.slug}`} className="footer__link">
              {p.name}
            </Link>
          ))}
          <Link to="/past-projects" className="footer__link">
            Past projects
          </Link>
        </nav>

        <nav className="footer__col" aria-label="Team">
          <h3 className="footer__heading">Team</h3>
          <Link to="/join" className="footer__link">
            Join us
          </Link>
          <a href={CONTACT.facebook} target="_blank" rel="noreferrer" className="footer__link">
            Facebook
          </a>
        </nav>

        <div className="footer__col">
          <h3 className="footer__heading">Contact</h3>
          <a href={`mailto:${CONTACT.email}`} className="footer__contact">
            <Mail size={16} /> {CONTACT.email}
          </a>
          <a href={CONTACT.facebook} target="_blank" rel="noreferrer" className="footer__contact">
            <Facebook size={16} /> /uwbiotron
          </a>
        </div>
      </div>

      <div className="container-wide footer__base">
        <span>© {new Date().getFullYear()} {CONTACT.org}</span>
        <span className="mono-label">Waterloo, ON · Canada</span>
      </div>
    </footer>
  );
}
