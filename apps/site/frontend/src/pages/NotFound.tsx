import { Link } from 'react-router-dom';
import { ArrowLeft } from 'lucide-react';
import PageMeta from '../components/PageMeta';

export default function NotFound() {
  return (
    <main id="main" className="notfound">
      <PageMeta
        title="Page not found | Biotron"
        description="We could not find the page you requested."
      />
      <div className="container notfound__inner">
        <span className="notfound__code mono-label">Error / 404</span>
        <h1 className="notfound__title">We could not find that page.</h1>
        <p className="notfound__body">It may have moved or never existed.</p>
        <Link to="/" className="notfound__cta">
          <ArrowLeft size={16} /> Back to home
        </Link>
      </div>
    </main>
  );
}
