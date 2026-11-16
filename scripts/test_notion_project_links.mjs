import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';

const html = readFileSync(new URL('../web/templates/index.html', import.meta.url), 'utf8');

test('every PM Day group has one direct Notion project URL', () => {
  const map = html.match(/const projectNotionUrls = \{([\s\S]*?)\n    \};/);
  assert.ok(map, 'project URL map should exist');

  const entries = [...map[1].matchAll(/(\d+): 'https:\/\/makeus-challenge\.notion\.site\/([0-9a-f-]+)'/g)];
  assert.equal(entries.length, 18);
  assert.deepEqual(entries.map(([, group]) => Number(group)).sort((a, b) => a - b), Array.from({ length: 18 }, (_, i) => i + 1));
  assert.equal(new Set(entries.map(([, , pageId]) => pageId)).size, 18);
});

test('project title link alerts before navigating to its Notion page', () => {
  assert.ok(/<a href="\$\{projectNotionUrls\[p\.group_no\]\}"[^>]*onclick="alert\(/.test(html), 'project title anchor should alert before opening its link');
});
