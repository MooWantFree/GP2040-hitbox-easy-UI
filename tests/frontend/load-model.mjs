import { readFile } from 'node:fs/promises';
const dataURL = (source) => `data:text/javascript;base64,${Buffer.from(source).toString('base64')}`;
const i18nURL = dataURL(await readFile(new URL('../../web/i18n.js', import.meta.url), 'utf8'));
export const i18n = await import(i18nURL);
export const languages = Object.fromEntries(await Promise.all(['en', 'zh', 'ja'].map(async (locale) => [locale, JSON.parse(await readFile(new URL(`../../locales/${locale}.json`, import.meta.url), 'utf8'))])));
i18n.configureLanguages({ languages, warnings: [] });
const source = await readFile(new URL('../../web/model.js', import.meta.url), 'utf8');
export const model = await import(dataURL(source.replace("'./i18n.js'", JSON.stringify(i18nURL))));
