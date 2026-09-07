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
      ? t('channels.signalLink.waiting', 'Waiting for linking...')
      : t('channels.signalLink.generating', 'Generating QR Code...')
    : '';

  useEffect(() => {
    if (!progressAnnouncement) {
      previousAnnouncementRef.current = '';
      return;
    }
    if (progressAnnouncement === previousAnnouncementRef.current) return;
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
          {t('channels.signalLink.generateQr', 'Generate QR Code')}
        </Button>
      </div>

      {(linkQR || linking) && (
        <div
          className="channels-page__qr-container"
          role="region"
          aria-label={t('channels.signalLink.regionLabel', 'Signal linking QR Code')}
        >
          {linkQR ? (
            <>
              <p className="channels-page__hint">
                {t('channels.signalLink.scanQr', 'Scan the QR Code with Signal on your phone:')}
              </p>
              <img
                src={linkQR}
                alt={t('channels.signalLink.qrAlt', 'QR Code to link Signal device')}
                className="channels-page__qr-image"
              />
            </>
          ) : (
            <p
              className="channels-page__hint"
            >
              {t('channels.signalLink.generating', 'Generating QR Code...')}
            </p>
          )}
          {linking && (
            <p
              className="channels-page__hint"
            >
              {t('channels.signalLink.waiting', 'Waiting for linking...')}
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
