// Account identity: reading the signed-in user and renaming them. Maps the
// backend's /me payload onto the UserProfile the account page and header
// chrome render.
import { apiGet, apiPatch } from '../api/client';
import { CurrentUserResponse } from '../api/types';
import { UserProfile } from '../types/nutrition';

const ME_PATH = '/api/v1/me';

/** Longest display name the backend accepts, mirrored so the input can cap
 *  typing instead of letting a save fail validation. */
export const DISPLAY_NAME_MAX_LENGTH = 60;

// A name seeded from an email local part reads as "jane.doe" rather than as
// two words, so the separators an address can carry count as word breaks too.
const initials = (name: string) => {
  const words = name.split(/[\s._-]+/).filter(Boolean);
  if (words.length === 0) return '?';
  if (words.length > 1) return `${words[0][0]}${words[1][0]}`.toUpperCase();
  return words[0].slice(0, 2).toUpperCase();
};

// The backend never sends a blank name, but this mapping runs inside the
// startup session check: throwing here would report a healthy session as a
// failed one. Fall back the way the server does instead.
const nameFor = (user: CurrentUserResponse) =>
  user.display_name?.trim() || user.email?.split('@')[0]?.trim() || 'Your account';

const toProfile = (user: CurrentUserResponse): UserProfile => {
  const name = nameFor(user);
  return {
    name,
    email: user.email ?? '',
    initials: initials(name),
    tag: user.roles?.includes('admin') ? 'admin' : 'signed in',
  };
};

export async function getCurrentUser(): Promise<UserProfile> {
  return toProfile(await apiGet<CurrentUserResponse>(ME_PATH));
}

export async function updateDisplayName(displayName: string): Promise<UserProfile> {
  return toProfile(await apiPatch<CurrentUserResponse>(ME_PATH, { display_name: displayName }));
}
