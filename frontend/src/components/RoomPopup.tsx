import { useRef } from 'react';
import { ArrowUpRight } from 'lucide-react';
import { Link } from 'react-router-dom';
import { gsap, useGSAP } from '../lib/gsap';
import { useScene } from '../lib/store';
import { PROJECTS, TEAM_STATS } from '../data/projects';
import CountUp from './CountUp';

/**
 * Glass info panel anchored to the side of the viewport. Its content swaps to
 * match the camera's active stop; it animates in/out as stops change. The 3D
 * model stays fully interactive behind/beside it.
 */
export default function RoomPopup() {
  const activeStop = useScene((s) => s.activeStop);
  const ready = useScene((s) => s.ready);
  const wrap = useRef<HTMLDivElement>(null);

  useGSAP(
    () => {
      if (!wrap.current) return;
      gsap.fromTo(
        wrap.current,
        { opacity: 0, y: 24, filter: 'blur(6px)' },
        { opacity: 1, y: 0, filter: 'blur(0px)', duration: 0.5, ease: 'power3.out' },
      );
    },
    { dependencies: [activeStop] },
  );

  if (!ready || !activeStop) return null;

  const project = PROJECTS.find((p) => p.modelId === activeStop);

  return (
    <div className="room-popup" ref={wrap} role="status" aria-live="polite">
      {activeStop === 'about' ? (
        <article className="glass popup-card">
          <span className="mono-label">00 / About</span>
          <h2 className="popup-title">We build machines that move people.</h2>
          <p className="popup-body">
            We are a University of Waterloo student team building powered exoskeletons, EMG
            wearables, and 3D-printed assistive devices.
          </p>
          <div className="popup-stats">
            {TEAM_STATS.map((s) => (
              <div key={s.label} className="popup-stat">
                <span className="popup-stat__num">
                  <CountUp value={s.value} suffix={s.suffix} plain={'plain' in s && s.plain} />
                </span>
                <span className="popup-stat__label">{s.label}</span>
              </div>
            ))}
          </div>
        </article>
      ) : project ? (
        <article className="glass popup-card">
          <span className="mono-label">{project.status}</span>
          <h2 className="popup-title">{project.name}</h2>
          <p className="popup-tagline">
            {project.tagline}
          </p>
          <p className="popup-body">{project.blurb}</p>
          <div className="popup-specs">
            {project.specs.map((sp) => (
              <div key={sp.label} className="popup-spec">
                <span className="popup-spec__val tabular">{sp.value}</span>
                <span className="popup-spec__lbl">{sp.label}</span>
              </div>
            ))}
          </div>
          <Link to={`/projects/${project.slug}`} className="popup-cta">
            View project <ArrowUpRight size={16} />
          </Link>
        </article>
      ) : null}
    </div>
  );
}
