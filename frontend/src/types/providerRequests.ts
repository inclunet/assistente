import { apidto } from '@wailsjs/go/models';

/**
 * `LLMProviders` é um bind Wails separado de `App`; o gerador não atualiza
 * estas classes com os DTOs internos. Mantemos o payload JSON tipado localmente
 * e o enviamos cru para o bridge, sem editar `wailsjs/` gerado.
 */
type DTOFields<T> = {
  [Key in keyof T as T[Key] extends (...args: never[]) => unknown ? never : Key]: T[Key];
};

type WithInsecureHTTPAuthorization<T> = Omit<DTOFields<T>, 'insecure_http_authorized_url'> & {
  insecure_http_authorized_url?: string;
};

export type CreateProviderRequest = WithInsecureHTTPAuthorization<apidto.CreateLLMProviderRequest>;
export type TestProviderRequest = WithInsecureHTTPAuthorization<apidto.TestLLMProviderRequest>;
export type UpdateProviderRequest = WithInsecureHTTPAuthorization<apidto.UpdateLLMProviderRequest>;

/** Wails recebe o objeto JSON; os métodos de instância das classes não viajam. */
export const toWailsRequest = <T,>(payload: object): T => payload as T;
