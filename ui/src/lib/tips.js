// Tips are fixed amounts, each with a name. These mirror tipOptions in tips.go on the server.
export const TIPS = [
  { a: 1, n: 'Suya', e: '\u{1F362}' },
  { a: 5, n: 'Burger', e: '\u{1F354}' },
  { a: 10, n: 'Pizza', e: '\u{1F355}' },
  { a: 25, n: 'Cake', e: '\u{1F370}' },
  { a: 50, n: 'Feast', e: '\u{1F357}' },
];
export const tipFor = amount => TIPS.find(t => t.a === amount);
