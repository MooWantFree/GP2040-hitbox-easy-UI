import { test } from 'node:test';
import assert from 'node:assert/strict';
import { releaseNotes } from '../../scripts/release-notes.mjs';

const changelog = '# Changelog\n\n## [Unreleased]\n\n- Future change\n\n## [0.2.0] - 2026-11-04\n\n### Changed\n\n- 顶部语言选择\n\n## [0.1.0-rc.1]\n\n- Preview\n';
test('extracts only the tagged version and preserves Markdown and Unicode', () => {
  assert.equal(releaseNotes(changelog, 'v0.2.0'), '### Changed\n\n- 顶部语言选择\n');
  assert.equal(releaseNotes(changelog.replaceAll('\n', '\r\n'), 'v0.1.0-rc.1'), '- Preview\n');
});
test('rejects missing, duplicate or empty notes and invalid tags', () => {
  for (const tag of ['main', 'v0.3.0', '0.2.0', 'v01.2.0', 'v0.2.0\nanything']) assert.throws(() => releaseNotes(changelog, tag));
  assert.throws(() => releaseNotes(changelog + '\n## [0.2.0]\n- Duplicate', 'v0.2.0'));
  assert.throws(() => releaseNotes('# Changelog\n## [0.2.0]\n### Changes\n\n## [0.1.0]\n- Old', 'v0.2.0'));
});
