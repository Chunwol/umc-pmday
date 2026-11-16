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

test('project title stays plain text and a visible button opens Notion without an alert', () => {
  assert.ok(/<h3[^>]*>\$\{escapeHtml\(p\.title\)\}<\/h3>/.test(html), 'project title should not be the only link');
  assert.ok(/<a href="\$\{projectNotionUrls\[p\.group_no\]\}" target="_blank" rel="noopener noreferrer"[^>]*>프로젝트 자세히 보기/.test(html), 'project card should have a clear Notion details link');
  assert.doesNotMatch(html, /onclick="alert\(/, 'project Notion links should not show an alert');
});
