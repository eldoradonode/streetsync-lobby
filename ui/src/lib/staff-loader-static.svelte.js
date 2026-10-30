// The test build has everything in one script, so the staff screens are simply there.
import StaffArea from '../components/StaffArea.svelte';

export const staff = { C: StaffArea, loading: false, failed: false };
export function ensureStaff() {}
