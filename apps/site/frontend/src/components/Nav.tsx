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

  // The drawer and its close button exist only up to 860px (see ui.css). If the
  // window grows past that while the menu is open, such as a tablet turned to
  // landscape, the close button vanishes but the scroll lock below would stay,
  // and the page could never scroll again. So the menu closes itself.
  useEffect(() => {
    if (!open) return;
    const narrow = window.matchMedia('(max-width: 860px)');
    const onChange = () => { if (!narrow.matches) setOpen(false); };
    narrow.addEventListener('change', onChange);
    return () => narrow.removeEventListener('change', onChange);
  }, [open]);

  useEffect(() => {
    if (!open) return;

    const root = document.documentElement;
    const previousOverflow = root.style.overflow;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setOpen(false);
    };

    root.style.overflow = 'hidden';
    window.addEventListener('keydown', onKeyDown);

    return () => {
      root.style.overflow = previousOverflow;
      window.removeEventListener('keydown', onKeyDown);
    };
  }, [open]);

  return (
    <header className={`nav ${scrolled ? 'nav--scrolled' : ''}`}>
      <div className="nav__inner container-wide">
        <Link to="/" className="nav__brand" aria-label="Biotron home" onClick={() => setOpen(false)}>
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
            How to join
          </Link>
        </nav>

        <button
          className="nav__burger"
          aria-label={open ? 'Close menu' : 'Open menu'}
          aria-expanded={open}
          aria-controls="mobile-navigation"
          onClick={() => setOpen((o) => !o)}
        >
          {open ? <X size={22} /> : <Menu size={22} />}
        </button>
      </div>

      {/* Mobile drawer */}
      <div
        id="mobile-navigation"
        // Lenis takes the wheel and touch for the whole page. This hands them back to the drawer.
        data-lenis-prevent
        className={`nav__drawer ${open ? 'nav__drawer--open' : ''}`}
        aria-hidden={!open}
      >
        {ITEMS.map((item) => (
          <NavLink
            key={item.label}
            to={item.to}
            className={({ isActive }) =>
              `nav__drawerlink ${isActive ? 'nav__drawerlink--active' : ''}`
            }
            tabIndex={open ? 0 : -1}
            onClick={() => setOpen(false)}
          >
            {item.label}
          </NavLink>
        ))}
        <Link
          to="/join"
          className="nav__cta nav__cta--block"
          tabIndex={open ? 0 : -1}
          onClick={() => setOpen(false)}
        >
          How to join
        </Link>
      </div>
    </header>
  );
}
