export const requiresRemoteHTTPAuthorization = (rawURL: string, authMode: string) => {
  if (authMode === 'none') return false;
  try {
    const url = new URL(rawURL);
    if (url.protocol !== 'http:') return false;
    const hostname = url.hostname.toLowerCase().replace(/^\[|\]$/g, '').replace(/\.$/, '');
    if (hostname === 'localhost' || hostname.endsWith('.localhost') || hostname === '::1') return false;
    const ipv4 = hostname.split('.').map(Number);
    return !(ipv4.length === 4 && ipv4.every(part => Number.isInteger(part) && part >= 0 && part <= 255) && ipv4[0] === 127);
  } catch {
    return false;
  }
};
