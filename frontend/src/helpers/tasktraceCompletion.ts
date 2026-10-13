// The stored percentage is manual; checking Done only overlays the displayed value.
export const COMPLETION_OPTIONS = Array.from({length: 11}, (_, index) => index * 10)
export function manualCompletion(value: unknown): number {
	const number = Number(value)
	return Number.isFinite(number) ? Math.max(0, Math.min(100, Math.round(number))) : 0
}
export function effectiveCompletion(value: unknown, done?: boolean): number {
	return done ? 100 : manualCompletion(value)
}
