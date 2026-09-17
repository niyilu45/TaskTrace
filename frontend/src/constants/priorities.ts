export const PRIORITIES = {
	'UNSET': 0,
	'LOW': 1,
	'MEDIUM': 2,
	'HIGH': 3,
	'URGENT': 4,
	'DO_NOW': 5,
} as const

// TaskTrace stores its 0–9 display range as 10–1; zero remains the legacy default.
export type Priority = typeof PRIORITIES[keyof typeof PRIORITIES] | 6 | 7 | 8 | 9 | 10
