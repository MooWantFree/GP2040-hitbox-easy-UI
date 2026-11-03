import { readFile, mkdir, writeFile } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { parseArgs } from 'node:util';

function sectionNotes(changelog, version, allowEmpty = false) {
  const text = changelog.replaceAll('\r\n', '\n');
  const headings = [...text.matchAll(/^##\s+(.+)$/gm)];
  const matches = headings.filter((heading) => heading[1].match(/^\[([^\]]+)\](?:\s+-\s+\d{4}-\d{2}-\d{2})?\s*$/)?.[1] === version);
  if (matches.length !== 1) throw new Error(`CHANGELOG.md must contain exactly one ## [${version}] section.`);
  const heading = matches[0];
  const next = headings[headings.indexOf(heading) + 1];
  const body = text.slice(heading.index + heading[0].length, next?.index ?? text.length).trim();
  if (!body || !body.split('\n').some((line) => line.trim() && !line.trim().startsWith('#'))) {
    if (allowEmpty) return 'Windows build from master.\n';
    throw new Error(`CHANGELOG.md has no release notes for v${version}.`);
  }
  return body + '\n';
}

export function releaseNotes(changelog, tag) {
  if (!/^v(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)(?:-[0-9A-Za-z]+(?:[.-][0-9A-Za-z]+)*)?$/.test(tag)) {
    throw new Error('Use a version tag such as v0.1.0 or v0.1.0-rc.1.');
  }
  return sectionNotes(changelog, tag.slice(1));
}

export function unreleasedNotes(changelog) {
  return sectionNotes(changelog, 'Unreleased', true);
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    const { values } = parseArgs({ options: { tag: { type: 'string' }, unreleased: { type: 'boolean' }, output: { type: 'string' }, changelog: { type: 'string', default: 'CHANGELOG.md' } } });
    if (values.unreleased && values.tag) throw new Error('Use either --unreleased or --tag, not both.');
    const changelog = await readFile(values.changelog, 'utf8');
    const notes = values.unreleased ? unreleasedNotes(changelog) : releaseNotes(changelog, values.tag || '');
    if (values.output) {
      await mkdir(dirname(values.output), { recursive: true });
      await writeFile(values.output, notes, 'utf8');
    } else process.stdout.write(notes);
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
