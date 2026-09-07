/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** Habilita logs de debug em qualquer build (ex.: "true" | "1"). Ver `utils/logger.ts`. */
  readonly VITE_DEBUG?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
