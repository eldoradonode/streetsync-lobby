// The admin console and moderator dashboard are a separate file that is downloaded the first time a
// moderator or admin opens one of them. Nobody else ever downloads it.
//
// Who may open them is still decided by the role (see showView in chat.svelte.js) and, for every request they make,
// by the server. This file only decides when the code is fetched. Browsers keep the file, so it is fetched once.
export const staff = $state({ C: null, loading: false, failed: false });

export function ensureStaff() {
  if (staff.C || staff.loading) return;
  staff.loading = true;
  staff.failed = false;
  import('../components/StaffArea.svelte')
    .then(m => { staff.C = m.default; })
    .catch(() => { staff.failed = true; }) // offline or a flaky connection: the page offers a retry
    .finally(() => { staff.loading = false; });
}
