import { describe, expect, it } from 'vitest';
import {
	appendImportedFileText,
	mergePastedShareList,
	normalizeSpaceSeparatedShareLinks,
} from './shareLinkListInput';

describe('normalizeSpaceSeparatedShareLinks', () => {
	it('splits space-separated share links into lines', () => {
		expect(normalizeSpaceSeparatedShareLinks('vless://a vless://b')).toBe(
			'vless://a\nvless://b',
		);
	});

	it('does not touch multiline JSON (no share schemes inside)', () => {
		const json = '{\n  "profiles": [ { "profileName": "default" } ]\n}';
		expect(normalizeSpaceSeparatedShareLinks(json)).toBe(json);
	});
});

describe('mergePastedShareList', () => {
	it('inserts normalized paste at caret', () => {
		const { next, caret } = mergePastedShareList('x', 1, 1, ' vless://a');
		expect(next).toBe('x\nvless://a');
		expect(caret).toBe(next.length);
	});
});

describe('appendImportedFileText', () => {
	it('replaces empty textarea with file content as-is', () => {
		const json = '{\n  "profiles": []\n}';
		expect(appendImportedFileText('', json)).toBe(json);
		expect(appendImportedFileText('  \n', json)).toBe(json);
	});

	it('appends to non-empty textarea on a new line', () => {
		expect(appendImportedFileText('vless://a', 'vless://b')).toBe('vless://a\nvless://b');
	});

	it('does not duplicate trailing newline of current text', () => {
		expect(appendImportedFileText('vless://a\n', 'vless://b')).toBe('vless://a\nvless://b');
	});

	it('strips leading BOM from file content', () => {
		expect(appendImportedFileText('', '\uFEFF{"profiles":[]}')).toBe('{"profiles":[]}');
	});

	it('keeps file content untrimmed otherwise (multiline JSON survives)', () => {
		const json = '{\n  "profiles": [\n    { "profileName": "default" }\n  ]\n}\n';
		expect(appendImportedFileText('', json)).toBe(json);
	});
});
