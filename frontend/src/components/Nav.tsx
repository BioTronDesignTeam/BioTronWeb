import { useEffect, useState } from 'react';
import { Link, NavLink, useLocation } from 'react-router-dom';
import { Menu, X } from 'lucide-react';
import { Brand } from '@biotron/style';

const ITEMS = [
  { label: 'Projects', to: '/projects' },
  { label: 'Sponsors', to: '/sponsors' },
  { label: 'Calendar', to: '/calendar' },
];

export default function Nav() {
  const [scrolled, setScrolled] = useState(false);
  const [open, setOpen] = useState(false);
  const location = useLocation();

  useEffect(() => {
    const onScroll = () => setScrolled(window.scrollY > 40);
    onScroll();
    window.addEventListener('scroll', onScroll, { passive: true });
    return () => window.removeEventListener('scroll', onScroll);
  }, []);

  useEffect(() => setOpen(false), [location.pathname]);

  return (
    <header className={`nav ${scrolled ? 'nav--scrolled' : ''}`}>
      <div className="nav__inner container-wide">
        <Link to="/" className="nav__brand" aria-label="Biotron home">
          <Brand label="" className="nav__logo" aria-hidden="true" />
        </Link>

        <nav className="nav__links" aria-label="Primary">
          {ITEMS.map((item) => (
            <NavLink
              key={item.label}
              to={item.to}
              className={({ isActive }) => `nav__link ${isActive ? 'nav__link--active' : ''}`}
            >
              {item.label}
            </NavLink>
          ))}
          <Link to="/join" className="nav__cta">
            Apply
          </Link>
        </nav>

        <button
          className="nav__burger"
          aria-label={open ? 'Close menu' : 'Open menu'}
          aria-expanded={open}
          onClick={() => setOpen((o) => !o)}
        >
          {open ? <X size={22} /> : <Menu size={22} />}
        </button>
      </div>

      {/* Mobile drawer */}
      <div className={`nav__drawer ${open ? 'nav__drawer--open' : ''}`} aria-hidden={!open}>
        {ITEMS.map((item) => (
          <NavLink
            key={item.label}
            to={item.to}
            className="nav__drawerlink"
            tabIndex={open ? 0 : -1}
          >
            {item.label}
          </NavLink>
        ))}
        <Link to="/join" className="nav__cta nav__cta--block" tabIndex={open ? 0 : -1}>
          Apply to join
        </Link>
      </div>
    </header>
  );
}
