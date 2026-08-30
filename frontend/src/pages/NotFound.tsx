import { Link } from 'react-router-dom';
import { ArrowLeft } from 'lucide-react';
import PageMeta from '../components/PageMeta';

export default function NotFound() {
  return (
    <main id="main" className="notfound">
      <PageMeta
        title="Page Not Found — Biotron"
        description="The requested Biotron page could not be found."
      />
      <div className="container notfound__inner">
        <span className="notfound__code mono-label">Error / 404</span>
        <h1 className="notfound__title">This page is still in prototype.</h1>
        <p className="notfound__body">The page you’re looking for doesn’t exist — or moved benches.</p>
        <Link to="/" className="notfound__cta">
          <ArrowLeft size={16} /> Back to home
        </Link>
      </div>
    </main>
  );
}
