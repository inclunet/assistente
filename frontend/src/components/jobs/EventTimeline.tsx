import { useTranslation } from 'react-i18next';
import { jobs } from '@wailsjs/go/models';
import './EventTimeline.css';

interface EventTimelineProps {
  events: jobs.EventEntry[];
  isLoading?: boolean;
}

function eventIcon(type: string): string {
  switch (type) {
    case 'triggered': return '⏰';
    case 'completed': return '✅';
    case 'failed': return '❌';
    case 'event_emitted': return '📡';
    case 'event_received': return '📥';
    default: return '•';
  }
}

export function EventTimeline({ events, isLoading }: EventTimelineProps) {
  const { t } = useTranslation();

  if (isLoading) {
    return <div className="event-timeline event-timeline--loading">{t('common.loading', 'Loading...')}</div>;
  }

  if (!events || events.length === 0) {
    return <div className="event-timeline event-timeline--empty">{t('jobs.eventsEmpty')}</div>;
  }

  return (
    <div className="event-timeline" role="log" aria-label={t('jobs.eventsTitle')}>
      {events.map((entry, i) => (
        <div key={`${entry.timestamp}-${i}`} className={`event-entry event-entry--${entry.type}`}>
          <span className="event-entry__icon" aria-hidden="true">{eventIcon(entry.type)}</span>
          <span className="event-entry__time">
            {entry.timestamp ? new Date(entry.timestamp).toLocaleTimeString() : ''}
          </span>
          <span className="event-entry__message">{entry.message || `${entry.type}: ${entry.job_id}`}</span>
          {entry.event && (
            <span className="event-entry__event">{entry.event}</span>
          )}
        </div>
      ))}
    </div>
  );
}
