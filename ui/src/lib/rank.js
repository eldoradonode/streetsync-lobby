// Tier names, short names and starting points. These mirror rank.go on the server.
export const TIERS = ['Newbie', 'Regular', 'Insider', 'Elite', 'Legend'];
export const ABBR = ['NEW', 'REG', 'INS', 'ELT', 'LGD'];
export const STARTS = [0, 300, 2000, 10000, 40000];
export const tierOf = r => Math.min(Math.max(r | 0, 0), 4);
