export function tasktracePriorityNumber(stored: number): number {
	return Number.isInteger(stored) && stored >= 1 && stored <= 10 ? 10 - stored : 9
}

export function tasktraceStoredPriority(displayed: number): number {
	if (!Number.isInteger(displayed) || displayed < 0 || displayed > 9) throw new RangeError('Priority must be between 0 and 9')
	return 10 - displayed
}
