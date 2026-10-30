import test from 'node:test';
import assert from 'node:assert/strict';
import { REACTIONS, validatePoll, percents, applyReaction, pinSnippet } from '../src/lib/engage.js';

test('reaction set is small and fixed', () => {
  assert.equal(REACTIONS.length, 6);
  assert.equal(new Set(REACTIONS).size, 6);
});

test('poll validation mirrors the server limits', () => {
  assert.equal(validatePoll('Best city?', ['Abuja', 'Lagos']).ok, true);
  assert.equal(validatePoll('ab', ['x', 'y']).ok, false);
  assert.equal(validatePoll('x'.repeat(141), ['x', 'y']).ok, false);
  assert.equal(validatePoll('Best city?', ['Abuja']).ok, false);
  assert.equal(validatePoll('Best city?', ['a', 'b', 'c', 'd', 'e', 'f', 'g']).ok, false);
  assert.equal(validatePoll('Best city?', ['Abuja', ' abuja ']).ok, false);
  assert.equal(validatePoll('Best city?', ['o'.repeat(61), 'y']).ok, false);
  const ok = validatePoll('  Best   city? ', [' Abuja ', '', 'Lagos']);
  assert.deepEqual([ok.ok, ok.q, ok.opts], [true, 'Best city?', ['Abuja', 'Lagos']]);
});

test('percentages add up to 100 and handle zero votes', () => {
  assert.deepEqual(percents([0, 0, 0]), [0, 0, 0]);
  assert.deepEqual(percents([1, 1, 1]).reduce((a, b) => a + b, 0), 100);
  assert.deepEqual(percents([2, 1]), [67, 33]);
  assert.deepEqual(percents([5]), [100]);
});

test('reaction frames update counts, and only my own frame changes my reaction', () => {
  const m = {};
  applyReaction(m, { i: 7, c: { '\u{1F44D}': 1 }, h: 'bob', y: '\u{1F44D}' }, 'ann');
  assert.deepEqual(m[7], { c: { '\u{1F44D}': 1 }, y: '' });
  applyReaction(m, { i: 7, c: { '\u{1F44D}': 2 }, h: 'Ann', y: '\u{1F44D}' }, 'ann');
  assert.equal(m[7].y, '\u{1F44D}');
  applyReaction(m, { i: 7, c: { '\u{1F44D}': 1 }, h: 'bob', y: '' }, 'ann');
  assert.equal(m[7].y, '\u{1F44D}');
  applyReaction(m, { i: 7, c: {}, h: 'ann', y: '' }, 'ann');
  assert.equal(m[7], undefined);
});

test('pin snippet is short and single line', () => {
  assert.equal(pinSnippet('a\nb   c'), 'a b c');
  assert.equal(Array.from(pinSnippet('x'.repeat(200))).length, 91);
});
