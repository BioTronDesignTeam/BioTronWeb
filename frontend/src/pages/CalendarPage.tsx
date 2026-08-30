import { CalendarDays, ArrowLeft } from 'lucide-react';
import { Link } from 'react-router-dom';
import PageMeta from '../components/PageMeta';

export default function CalendarPage() {
  return (
    <main id="main" className="calendarpage">
      <PageMeta
        title="Calendar | Biotron"
        description="Biotron’s public events calendar and subscription feed are coming soon."
      />
      <div className="container calendarpage__inner">
        <div className="calendarpage__icon" aria-hidden="true">
          <CalendarDays size={28} />
        </div>
        <span className="eyebrow mono-label">Public calendar</span>
        <h1>Our public calendar is coming soon.</h1>
        <p>
          You will be able to find Biotron events here and subscribe from your calendar app.
        </p>
        <Link to="/" className="calendarpage__back">
          <ArrowLeft size={16} /> Back home
        </Link>
      </div>
    </main>
  );
}
