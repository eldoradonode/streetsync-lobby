// Small pictures for rooms, drawn by the device, so nothing is downloaded.
const BY_SLUG = {
  lobby: '\u{1F4AC}', abuja: '\u{1F3D9}\uFE0F', lagos: '\u{1F306}', campus: '\u{1F393}', football: '\u{1F3C6}',
  music: '\u{1F3B5}', tech: '\u{1F4BB}', 'side-hustle': '\u{1F4BC}', creators: '\u{1F3A8}', 'elite-lounge': '\u{1F451}',
};
const BY_CATEGORY = {
  General: '\u{1F4AC}', Cities: '\u{1F3D9}\uFE0F', Education: '\u{1F393}', Sports: '\u{1F3C6}', Entertainment: '\u{1F3B5}',
  Tech: '\u{1F4BB}', Business: '\u{1F4BC}', Creators: '\u{1F3A8}', Members: '\u{1F451}', Community: '\u{1F5E8}\uFE0F',
};
export const roomIcon = r => BY_SLUG[r.s] || BY_CATEGORY[r.c] || '\u{1F5E8}\uFE0F';
