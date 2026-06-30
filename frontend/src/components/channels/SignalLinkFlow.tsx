import { useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '../index';
import { useAnnouncer } from '../../hooks/useAnnouncer';

interface SignalLinkFlowProps {
  apiURL: string;
  linkQR: string;
  linking: boolean;
  onLink: () => Promise<void>;
  onReset: () => void;
}

export function SignalLinkFlow({
  apiURL,
  linkQR,
  linking,
  onLink,
  onReset,
}: SignalLinkFlowProps) {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const previousAnnouncementRef = useRef('');
  const progressAnnouncement = linking
    ? linkQR
      ? t('channels.signalLink.waiting')
      : t('channels.signalLink.generating')
    : '';

  useEffect(() => {
    if (!progressAnnouncement || progressAnnouncement === previousAnnouncementRef.current) return;
    announce(progressAnnouncement);
    previousAnnouncementRef.current = progressAnnouncement;
  }, [announce, progressAnnouncement]);

  return (
    <div className="channels-page__fields">
      <div className="channels-page__row">
        <Button
          variant="outline"
          onClick={onLink}
          disabled={!apiURL || linking}
          loading={linking}
        >
          {t('channels.signalLink.generateQr')}
        </Button>
      </div>

      {(linkQR || linking) && (
        <div
          className="channels-page__qr-container"
          role="region"
          aria-label={t('channels.signalLink.regionLabel')}
        >
          {linkQR ? (
            <>
              <p className="channels-page__hint">
                {t('channels.signalLink.scanQr')}
              </p>
              <img
                src={linkQR}
                alt={t('channels.signalLink.qrAlt')}
                className="channels-page__qr-image"
              />
            </>
          ) : (
            <p
              className="channels-page__hint"
            >
              {t('channels.signalLink.generating')}
            </p>
          )}
          {linking && (
            <p
              className="channels-page__hint"
            >
              {t('channels.signalLink.waiting')}
            </p>
          )}
          <Button variant="ghost" onClick={onReset}>
            {t('common.cancel')}
          </Button>
        </div>
      )}
    </div>
  );
}
