/**
 * Extract a human-readable message from an unknown thrown value.
 *
 * `catch` clauses bind their variable as `unknown` under TS strict mode, so a
 * thrown value is not guaranteed to be an `Error`. This narrows it: a real
 * `Error` with a non-empty message yields that message, everything else (empty
 * message, string throw, plain object, `undefined`) falls back to `fallback`.
 */
export function errorMessage(e: unknown, fallback = 'Ошибка'): string {
	if (e instanceof Error && e.message) {
		return e.message;
	}
	if (typeof e === 'string' && e) {
		return e;
	}
	return fallback;
}
