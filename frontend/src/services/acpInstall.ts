import { UpdateACPAgent } from '@wailsjs/go/wailsapi/ACPInstall';
import type { apidto } from '@wailsjs/go/models';

type InstallPlan = apidto.ACPInstallPlan;

/**
 * Pede ao backend a atualização descrita pelo plano que ele próprio forneceu.
 * O consentimento para artefato não verificado é sempre explícito.
 */
export const requestACPAgentUpdate = (
  plan: InstallPlan,
  acceptUnverified: boolean,
) => UpdateACPAgent(plan.agent_id, {
  distribution: plan.distribution || '',
  origin: plan.origin || '',
  sha256: plan.sha256 || '',
  accept_unverified: acceptUnverified,
});
