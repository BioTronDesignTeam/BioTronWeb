import { Suspense, lazy } from 'react';
import { useParams, Link } from 'react-router-dom';
import { ArrowLeft, ArrowUpRight, Rotate3d } from 'lucide-react';
import { getProject, PROJECTS, CONTACT } from '../data/projects';
import Reveal from '../components/Reveal';
import SplitText from '../components/SplitText';
import ModelFallback from '../components/ModelFallback';
import { useReducedMotion, useWebGLSupported } from '../lib/hooks';
import NotFound from './NotFound';
import PageMeta from '../components/PageMeta';

const ProjectViewer = lazy(() => import('../three/ProjectViewer'));

export default function ProjectDetail() {
  const { slug } = useParams();
  const project = slug ? getProject(slug) : undefined;
  const reduced = useReducedMotion();
  const webgl = useWebGLSupported();

  if (!project) return <NotFound />;

  const others = PROJECTS.filter((p) => p.slug !== project.slug);
  const showLive = webgl && !reduced;

  return (
    <main id="main" className="detail" style={{ ['--card-accent' as string]: project.accentHex }}>
      <PageMeta title={`${project.name} — Biotron`} description={project.blurb} />
      <div className="container detail__top">
        <Link to="/projects" className="detail__back" data-cursor>
          <ArrowLeft size={16} /> All projects
        </Link>
      </div>

      <section className="detail__hero container">
        <div className="detail__intro">
          <span className="mono-label">{project.status}</span>
          <SplitText as="h1" className="detail__title" text={project.name} by="char" />
          <p className="detail__tagline">
            {project.tagline}
          </p>
          <p className="detail__blurb">{project.blurb}</p>
          <div className="detail__specs">
            {project.specs.map((sp) => (
              <div key={sp.label} className="detail__spec glass">
                <span className="detail__specval tabular">{sp.value}</span>
                <span className="detail__speclbl">{sp.label}</span>
              </div>
            ))}
          </div>
        </div>

        <div className="detail__viewer glass">
          {showLive ? (
            <Suspense fallback={<div className="detail__viewerloading" />}>
              <ProjectViewer id={project.modelId} color={project.accentHex} />
            </Suspense>
          ) : (
            <ModelFallback project={project} />
          )}
          {showLive && (
            <span className="detail__viewerhint">
              <Rotate3d size={14} /> Drag to rotate
            </span>
          )}
        </div>
      </section>

      <section className="detail__body container">
        <div className="detail__story">
          <Reveal>
            <span className="eyebrow mono-label">Overview</span>
          </Reveal>
          {project.description.map((para, i) => (
            <Reveal key={i} delay={i * 0.05}>
              <p className="detail__para">{para}</p>
            </Reveal>
          ))}
        </div>

        <aside className="detail__side">
          <Reveal className="detail__sidecard glass">
            <h3 className="detail__sidehead">Sub-teams involved</h3>
            <ul className="detail__teamlist">
              {project.team.map((t) => (
                <li key={t}>{t}</li>
              ))}
            </ul>
            <a href={`mailto:${CONTACT.email}?subject=${project.name}`} className="detail__sidecta">
              Get involved <ArrowUpRight size={16} />
            </a>
          </Reveal>
        </aside>
      </section>

      <section className="detail__next container">
        <h2 className="detail__nexttitle">More projects</h2>
        <div className="detail__nextgrid">
          {others.map((p) => (
            <Link
              key={p.slug}
              to={`/projects/${p.slug}`}
              className="detail__nextcard glass"
              style={{ ['--card-accent' as string]: p.accentHex }}
            >
              <span className="mono-label">{p.status}</span>
              <h3>{p.name}</h3>
              <p>{p.tagline}</p>
              <span className="detail__nextarrow">
                <ArrowUpRight size={18} />
              </span>
            </Link>
          ))}
        </div>
      </section>
    </main>
  );
}
