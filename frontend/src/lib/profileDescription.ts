import type { TFunction } from 'i18next';

export interface DescribedProfile {
  slug: string;
  description?: string;
  builtin?: boolean;
}

export function profileDisplayDescription(t: TFunction, profile: DescribedProfile): string {
  const description = profile.description ?? '';
  if (!profile.builtin) {
    return description;
  }
  return t(`profiles.builtinDescriptions.${profile.slug}`, description);
}
