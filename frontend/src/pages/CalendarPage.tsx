import { CalendarDays, ArrowLeft, ArrowUpRight } from 'lucide-react';
import { Link } from 'react-router-dom';
import PageMeta from '../components/PageMeta';
import { CONTACT } from '../data/projects';

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
        <p className="calendarpage__current">
          Looking for the next meeting? Discord has the current time and room while we connect the
          public feed.
        </p>
        <div className="calendarpage__actions">
          <a href={CONTACT.discord} target="_blank" rel="noreferrer" className="calendarpage__primary">
            Join Discord <ArrowUpRight size={16} />
          </a>
          <Link to="/" className="calendarpage__back">
            <ArrowLeft size={16} /> Back home
          </Link>
        </div>
      </div>
    </main>
  );
}
