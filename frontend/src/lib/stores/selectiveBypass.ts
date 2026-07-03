/**
 * Lightweight store for selective-bypass status and rebuild progress.
 * Populated via SSE events (singbox-router:selective-status and
 * singbox-router:selective-progress) and REST polling on drawer open.
 */
import { writable } from 'svelte/store';
import type { SelectiveStatus, SelectiveProgress } from '$lib/types';

function createSelectiveBypassStore() {
	const status = writable<SelectiveStatus | null>(null);
	const progress = writable<SelectiveProgress | null>(null);
	// Set to true by explicit triggers (Apply button, engine enable) to request
	// the global modal open. The modal clears this on close.
	const modalRequested = writable(false);
	// Monotonic counter bumped on every applyStatus. Lets callers detect that
	// a NEWER status (e.g. a terminal SSE event) landed while their request
	// was in flight, so they can skip applying a stale response body.
	let epoch = 0;

	return {
		status,
		progress,
		modalRequested,
		applyStatus(data: SelectiveStatus) {
			epoch++;
			status.set(data);
		},
		statusEpoch(): number {
			return epoch;
		},
		applyProgress(data: SelectiveProgress) {
			progress.set(data);
			// Progress is cleared only when the modal closes (resetProgress), not on a
			// timer — otherwise the UI reverts to empty pending steps while the user
			// is still reading the domain→IP snapshot.
		},
		resetProgress() {
			progress.set(null);
		},
		/** Explicitly request the rebuild modal to open (called by Apply / engine enable). */
		requestModal() {
			modalRequested.set(true);
		},
		clearModalRequest() {
			modalRequested.set(false);
		},
	};
}

export const selectiveBypass = createSelectiveBypassStore();
