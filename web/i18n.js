let catalog = { languages: { en: { name: 'English', messages: {} } }, warnings: [] };
let locale = 'en';
const own = (object, key) => Object.hasOwn(object || {}, key);

export function resolveLanguage(preferred, languages) {
  for (const requested of preferred) {
    let candidate;
    try { candidate = Intl.getCanonicalLocales(requested.replaceAll('_', '-'))[0]?.toLowerCase(); } catch { continue; }
    while (candidate) {
      if (own(languages, candidate)) return candidate;
      const at = candidate.lastIndexOf('-');
      candidate = at < 0 ? '' : candidate.slice(0, at);
    }
  }
  return 'en';
}
export function configureLanguages(value, preferred = ['en']) {
  catalog = value;
  locale = resolveLanguage(preferred, catalog.languages);
  return locale;
}
export function setLanguage(preferred) { locale = resolveLanguage(preferred, catalog.languages); return locale; }
export const currentLanguage = () => locale;
export const availableLanguages = () => Object.entries(catalog.languages).map(([locale, pack]) => ({ locale, name: pack.name }));
export const languageWarnings = () => catalog.warnings || [];
export function t(key, ...args) {
  const local = catalog.languages[locale]?.messages;
  const english = catalog.languages.en?.messages;
  const value = own(local, key) ? local[key] : own(english, key) ? english[key] : key;
  return value.replace(/\{(\d+)\}/g, (token, index) => index < args.length ? String(args[index]) : token);
}
export function translateError(value) {
  if (value && typeof value === 'object' && typeof value.key === 'string') return t(value.key, ...(value.args || []).map(translateError));
  return value == null ? '' : String(value);
}
export class I18nError extends Error {
  constructor(key, ...args) {
    super();
    Object.defineProperty(this, 'message', { get: () => t(key, ...args.map((arg) => arg instanceof Error ? errorText(arg) : arg)) });
  }
}
export const errorText = (error) => error.errorMessage ? translateError(error.errorMessage) : error.message;
export function translatePage(root = document) {
  root.documentElement?.setAttribute('lang', locale);
  for (const el of root.querySelectorAll('[data-i18n]')) el.textContent = t(el.dataset.i18n);
  for (const attr of ['aria-label','title','placeholder']) {
    for (const el of root.querySelectorAll(`[data-i18n-${attr}]`)) el.setAttribute(attr, t(el.getAttribute(`data-i18n-${attr}`)));
  }
}
export async function loadLanguages(preferred) {
  const response = await fetch('/api/locales', { cache: 'no-store' });
  if (!response.ok) throw new Error('Could not load language files. Refresh to retry.');
  return configureLanguages(await response.json(), preferred);
}
