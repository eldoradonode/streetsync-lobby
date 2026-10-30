import test from 'node:test';
import assert from 'node:assert/strict';
import { applyMember, mergeMembers, groupMembers, directoryOrder, STATUS, SORTS } from '../src/lib/rooms20.js';

test('status labels never rely on colour alone', () => {
  const marks = new Set(Object.values(STATUS).map(s => s.mark));
  const labels = new Set(Object.values(STATUS).map(s => s.label));
  assert.equal(marks.size, 3);
  assert.equal(labels.size, 3);
  assert.ok(STATUS.on.label === 'Online' && STATUS.off.label === 'Offline');
});

test('presence frames add, update and remove people', () => {
  const l = [{ h: 'Ann', s: 'off', a: '', o: '', x: false }];
  assert.equal(applyMember(l, { h: 'ann', s: 'on' }), 0);
  assert.equal(l[0].s, 'on');
  assert.equal(applyMember(l, { h: 'Bob', s: 'on', a: 'f3' }), 1);
  assert.equal(l.length, 2);
  assert.equal(applyMember(l, { h: 'BOB', d: 1 }), -1);
  assert.equal(applyMember(l, { h: 'nobody', d: 1 }), 0);
  assert.equal(applyMember(l, { h: 'Cy' }), 0); // no status, nothing to show
  assert.equal(l.length, 1);
});

test('merging a page never repeats anyone', () => {
  const l = [{ h: 'Ann', s: 'on' }];
  mergeMembers(l, [{ h: 'ann', s: 'off' }, { h: 'Bob', s: 'off' }]);
  assert.deepEqual(l.map(x => x.h), ['Ann', 'Bob']);
});

test('groups: online, hidden, offline, host first', () => {
  const g = groupMembers([{ h: 'zed', s: 'on' }, { h: 'amy', s: 'on' }, { h: 'host', s: 'on', x: true }, { h: 'q', s: 'hid' }, { h: 'o', s: 'off' }]);
  assert.deepEqual(g.on.map(x => x.h), ['host', 'amy', 'zed']);
  assert.equal(g.hid.length, 1);
  assert.equal(g.off.length, 1);
});

test('directory keeps the server order and never sorts in the browser', () => {
  const rooms = [{ s: 'lobby' }, { s: 'a', k: 'public' }, { s: 'mine', k: 'private' }];
  const dir = [{ s: 'c', k: 'public' }, { s: 'a', k: 'public' }, { s: 'b', k: 'public' }];
  assert.deepEqual(directoryOrder(rooms, dir).map(r => r.s), ['lobby', 'c', 'a', 'b', 'mine']);
  assert.deepEqual(directoryOrder(rooms, []).map(r => r.s), ['lobby', 'a', 'mine']);
  assert.equal(SORTS.length, 3);
});
