import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFile, readdir } from 'node:fs/promises';
import { i18n, languages, model } from './load-model.mjs';
const reset = (preferred = ['en']) => i18n.configureLanguages({ languages, warnings: [] }, preferred);

test('browser language priority, region matching and English fallback', () => {
  for (const [preferred, want] of [
    [['ja-JP', 'en-US'], 'ja'], [['zh-Hant-TW'], 'zh'], [['zh_CN'], 'zh'],
    [['de-DE', 'ja-JP'], 'ja'], [['en-GB', 'zh-CN'], 'en'], [['fr-FR'], 'en'], [[], 'en'],
  ]) assert.equal(reset(preferred), want);
  reset();
});

test('partial custom pack, regional locale and missing entries fall back to English', () => {
  const extra = { ...languages, 'pt-br': { name: 'Português', messages: { 'page.title': 'Botões', 'layout.bound': '{1} total, {0} identificados' } } };
  i18n.configureLanguages({ languages: extra }, ['pt-BR']);
  assert.equal(i18n.currentLanguage(), 'pt-br');
  assert.equal(i18n.t('page.title'), 'Botões');
  assert.equal(i18n.t('layout.bound', 3, 16), '16 total, 3 identificados');
  assert.equal(i18n.t('connection.connect'), 'Connect');
  assert.equal(i18n.setLanguage(['unknown-language']), 'en');
  reset();
});

test('language changes translate nested errors without changing layouts or mapping values', () => {
  const layout = { version: 2, name: 'My layout 自分', deviceKey: 'device', labelSet: 'playstation', buttons: [{ id: 'one', name: '自定义键', x: 25, y: 50, size: 7, shape: 'circle', pin: 3 }] };
  const mapping = model.makeMapping([5, 11, 16, 1]);
  const before = JSON.stringify({layout, mapping});
  const error = Object.assign(new Error('English fallback'), { errorMessage: { key: 'error.backupAborted', args: [{ key: 'error.layoutSave', args: ['disk full'] }] } });
  const wrapper = new i18n.I18nError('connection.webMode', error);
  for (const locale of ['en', 'zh', 'ja']) {
    i18n.setLanguage([locale]);
    assert.equal(i18n.errorText(error), i18n.t('error.backupAborted', i18n.t('error.layoutSave', 'disk full')));
    assert.equal(wrapper.message, i18n.t('connection.webMode', i18n.errorText(error)));
    assert.equal(model.mappingLabel(null), i18n.t('identify.unbound'));
    assert.equal(model.DIRECTIONS.up, i18n.t('direction.up'));
    assert.equal(model.mappingLabel(mapping, 'playstation'), '↑ + × + L2 + Touchpad');
    assert.deepEqual(model.validateLayout(layout), layout);
    assert.equal(JSON.stringify({layout, mapping}), before);
    assert.equal(model.isExtendedMapping({ action: 40, customButtonMask: 32768, customDpadMask: 0 }), true);
    assert.equal(model.isExtendedMapping(mapping), false);
  }
  reset();
});

test('translations are applied as text and attributes, never markup', () => {
  i18n.configureLanguages({ languages: { ...languages, fr: { name: 'Français', messages: { 'page.title': '<img src=x onerror=alert(1)>' } } } }, ['fr']);
  const textNode = { dataset: { i18n: 'page.title' }, textContent: '' };
  const attrs = {};
  const attrNode = { getAttribute: () => 'page.title', setAttribute: (key, value) => { attrs[key] = value; } };
  const root = { documentElement: { setAttribute: (key, value) => { attrs[key] = value; } }, querySelectorAll: (selector) => selector === '[data-i18n]' ? [textNode] : [attrNode] };
  i18n.translatePage(root);
  assert.equal(textNode.textContent, '<img src=x onerror=alert(1)>');
  assert.equal(attrs['aria-label'], textNode.textContent);
  assert.equal(attrs.lang, 'fr');
  assert.equal(Object.hasOwn(textNode, 'innerHTML'), false);
  reset();
});

test('every translation key referenced in the UI and backend exists in all shipped languages', async () => {
  const backend = (await readdir(new URL('../../internal/app/', import.meta.url))).filter((name) => name.endsWith('.go') && !name.endsWith('_test.go')).map((name) => `internal/app/${name}`);
  const files = ['web/app.js', 'web/model.js', 'web/index.html', ...backend];
  const pattern = /(?:\b(?:t|I18nError|nativeText|messageError)\(['"]([^'"]+)['"]|data-i18n(?:-(?:aria-label|title|placeholder))?="([^"]+)")/g;
  for (const filename of files) {
    const source = await readFile(new URL(`../../${filename}`, import.meta.url), 'utf8');
    for (const match of source.matchAll(pattern)) for (const [locale, pack] of Object.entries(languages)) {
      assert.ok(pack.messages[match[1] || match[2]], `${filename}: missing ${locale} translation ${match[1] || match[2]}`);
    }
  }
});
