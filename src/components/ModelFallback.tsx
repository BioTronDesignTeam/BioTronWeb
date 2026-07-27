import type { Project } from '../data/projects';

/**
 * Lightweight static visual used in place of the live 3D model in the
 * reduced-motion / no-WebGL stacked layout. Pure CSS/SVG — no canvas.
 */
export default function ModelFallback({ project }: { project: Project }) {
  return (
    <div
      className="model-fallback"
      style={{ ['--mf-accent' as string]: project.accentHex }}
      aria-hidden="true"
    >
      <svg viewBox="0 0 200 200" role="presentation">
        <defs>
          <radialGradient id={`g-${project.slug}`} cx="50%" cy="45%" r="60%">
            <stop offset="0%" stopColor={project.accentHex} stopOpacity="0.5" />
            <stop offset="100%" stopColor={project.accentHex} stopOpacity="0" />
          </radialGradient>
        </defs>
        <circle cx="100" cy="95" r="80" fill={`url(#g-${project.slug})`} />
        <g stroke={project.accentHex} strokeWidth="1.5" fill="none" opacity="0.9">
          <circle cx="100" cy="95" r="48" strokeDasharray="4 6" />
          <circle cx="100" cy="95" r="30" />
          <path d="M100 47 L100 143 M52 95 L148 95" strokeOpacity="0.4" />
        </g>
        <circle cx="100" cy="95" r="6" fill={project.accentHex} />
      </svg>
    </div>
  );
}
