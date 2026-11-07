import { useEffect, useRef } from "react";
import Reveal from "../components/Reveal";
import { SPONSORS } from "../data/sponsors";

/**
 * The marquee runs two identical halves so the loop has no seam. Each half
 * repeats the list so it stays wider than the widest screen; a shorter half
 * leaves a blank stretch at the end of every loop.
 */
// Two halves of four copies each.
const MARQUEE_COPIES = 8;
const MARQUEE_ROW = Array.from({ length: MARQUEE_COPIES }, (_, copy) =>
  SPONSORS.map((sponsor) => ({ key: `${copy}:${sponsor.name}`, name: sponsor.name })),
).flat();

/** Pixels per second. The loop time follows the track width, so the speed holds when sponsors change. */
const MARQUEE_SPEED = 40;

export default function Sponsors() {
  const trackRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const track = trackRef.current;
    if (!track) return;
    const setDuration = () => {
      track.style.animationDuration = `${track.scrollWidth / 2 / MARQUEE_SPEED}s`;
    };
    setDuration();
    // Fonts and logos load after the first paint and change the width.
    const observer = new ResizeObserver(setDuration);
    observer.observe(track);
    return () => observer.disconnect();
  }, []);

  return (
    <section className="sponsors" aria-label="Partners and sponsors">
      <div className="container">
        <Reveal>
          <span className="mono-label">Backed by</span>
        </Reveal>
      </div>
      <div className="sponsors__marquee" aria-hidden="true">
        <div className="sponsors__track" ref={trackRef}>
          {MARQUEE_ROW.map((item) => (
            // Names only. The logos are made for light backgrounds and this strip is dark.
            <span key={item.key} className="sponsors__item">
              {item.name}
            </span>
          ))}
        </div>
      </div>
    </section>
  );
}
