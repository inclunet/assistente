import { Button } from '../index';

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
  return (
    <div className="channels-page__fields">
      <div className="channels-page__row">
        <Button
          variant="outline"
          onClick={onLink}
          disabled={!apiURL || linking}
          loading={linking}
        >
          Gerar QR Code
        </Button>
      </div>

      {(linkQR || linking) && (
        <div
          className="channels-page__qr-container"
          role="region"
          aria-label="QR Code de vinculação Signal"
        >
          {linkQR ? (
            <>
              <p className="channels-page__hint">
                Escaneie o QR Code com o Signal no celular:
              </p>
              <img
                src={linkQR}
                alt="QR Code para vincular dispositivo Signal"
                className="channels-page__qr-image"
              />
            </>
          ) : (
            <p
              className="channels-page__hint"


            >
              Gerando QR Code...
            </p>
          )}
          {linking && (
            <p
              className="channels-page__hint"


            >
              Aguardando vinculação...
            </p>
          )}
          <Button variant="ghost" onClick={onReset}>
            Cancelar
          </Button>
        </div>
      )}
    </div>
  );
}
