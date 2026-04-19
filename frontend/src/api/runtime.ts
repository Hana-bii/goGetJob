const DEFAULT_API_BASE_URL = import.meta.env.DEV ? 'http://localhost:8080' : '';

function trimTrailingSlash(value: string): string {
  return value.replace(/\/+$/, '');
}

function isAbsoluteUrl(value: string): boolean {
  return /^[a-zA-Z][a-zA-Z\d+\-.]*:\/\//.test(value);
}

function normalizeHttpUrlToWebSocketUrl(value: string): string {
  if (value.startsWith('http://') || value.startsWith('https://')) {
    return value.replace(/^http/i, 'ws');
  }

  if (value.startsWith('//')) {
    const protocol = typeof window !== 'undefined' && window.location.protocol === 'https:'
      ? 'wss:'
      : 'ws:';
    return `${protocol}${value}`;
  }

  if (value.startsWith('ws://') && typeof window !== 'undefined' && window.location.protocol === 'https:') {
    return value.replace(/^ws:/i, 'wss:');
  }

  return value;
}

function resolveRelativeWebSocketUrl(path: string): string {
  if (typeof window === 'undefined') {
    return path;
  }

  const origin = window.location.origin.replace(/^http/i, 'ws');
  return `${origin}${path.startsWith('/') ? path : `/${path}`}`;
}

const apiBaseUrl = trimTrailingSlash(import.meta.env.VITE_API_BASE_URL?.trim() || DEFAULT_API_BASE_URL);
const wsBaseUrl = trimTrailingSlash(import.meta.env.VITE_WS_BASE_URL?.trim() || '');

export const API_BASE_URL = apiBaseUrl;
export const WS_BASE_URL = wsBaseUrl;

export function buildApiUrl(path: string): string {
  if (!API_BASE_URL) {
    return path;
  }

  return `${API_BASE_URL}${path.startsWith('/') ? path : `/${path}`}`;
}

export function buildWebSocketUrl(path: string): string {
  const normalizedPath = path.startsWith('/') ? path : `/${path}`;

  if (WS_BASE_URL) {
    return normalizeHttpUrlToWebSocketUrl(`${WS_BASE_URL}${normalizedPath}`);
  }

  return resolveRelativeWebSocketUrl(normalizedPath);
}

export function resolveWebSocketUrl(webSocketUrl: string | null | undefined, fallbackPath: string): string {
  const candidate = webSocketUrl?.trim();
  if (candidate) {
    return isAbsoluteUrl(candidate) || candidate.startsWith('//')
      ? normalizeHttpUrlToWebSocketUrl(candidate)
      : buildWebSocketUrl(candidate);
  }

  return buildWebSocketUrl(fallbackPath);
}


