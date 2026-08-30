import { CalendarDays, ArrowLeft } from 'lucide-react';
import { Link } from 'react-router-dom';
import PageMeta from '../components/PageMeta';

export default function CalendarPage() {
  return (
    <main id="main" className="calendarpage">
      <PageMeta
        title="Calendar — Biotron"
        description="BioTron’s public events calendar and subscription feeds are coming soon."
      />
      <div className="container calendarpage__inner">
        <div className="calendarpage__icon" aria-hidden="true">
          <CalendarDays size={28} />
        </div>
        <span className="eyebrow mono-label">Public calendar</span>
        <h1>Our events are still being wired in.</h1>
        <p>
          This will become the home for BioTron events and subscribable calendar feeds. The
          calendar service is still on the workbench, so check back after the next build.
        </p>
        <Link to="/" className="calendarpage__back">
          <ArrowLeft size={16} /> Back to the workshop
        </Link>
      </div>
    </main>
  );
}
