// The sections of the two staff areas. The same list builds the sidebar on desktop and the tab strip on phones.
export const SECTIONS = {
  admin: [
    { id: 'overview', label: 'Overview', icon: 'chart' },
    { id: 'users', label: 'Users', icon: 'users' },
    { id: 'moderators', label: 'Moderators', icon: 'shield' },
    { id: 'rooms', label: 'Rooms', icon: 'rooms' },
    { id: 'reports', label: 'Reports', icon: 'flag' },
    { id: 'log', label: 'Audit log', icon: 'list' },
  ],
  mod: [
    { id: 'overview', label: 'Overview', icon: 'chart' },
    { id: 'reports', label: 'Reports', icon: 'flag' },
    { id: 'users', label: 'Users', icon: 'users' },
    { id: 'rooms', label: 'Rooms', icon: 'rooms' },
  ],
};
export const AREA_NAME = { admin: 'Admin console', mod: 'Moderation' };
