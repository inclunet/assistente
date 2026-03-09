export function isSafeLinkHref(href: string): boolean {
  const raw = String(href ?? '').trim();
  if (!raw) return false;

  // Âncoras locais
  if (raw.startsWith('#')) return true;

  // Bloqueia caracteres de controle e espaços estranhos.
  if (/[\u0000-\u001F\u007F]/.test(raw)) return false;

  // Normaliza e avalia protocolo.
  try {
    // Base só para permitir parse de URLs relativas.
    const url = new URL(raw, 'https://example.invalid');

    // Se veio relativo, o protocolo será https: (da base) e o href original não tem esquema.
    const hasExplicitScheme = /^[a-zA-Z][a-zA-Z0-9+.-]*:/.test(raw);
    if (!hasExplicitScheme) {
      // Permite relativos (./ ../ /) e paths simples.
      return true;
    }

    const protocol = url.protocol.toLowerCase();
    return protocol === 'http:' || protocol === 'https:' || protocol === 'mailto:';
  } catch {
    return false;
  }
}
