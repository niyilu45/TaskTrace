import {describe, expect, it} from 'vitest'
import {tasktracePriorityNumber, tasktraceStoredPriority} from './tasktracePriority'

describe('TaskTrace priority mapping', () => {
	it('shows legacy unset and new default priority as 9', () => {
		expect(tasktracePriorityNumber(0)).toBe(9)
		expect(tasktracePriorityNumber(1)).toBe(9)
	})

	it('preserves smaller-is-higher display order using the existing descending database order', () => {
		for (let priority = 0; priority <= 9; priority++) {
			const stored = tasktraceStoredPriority(priority)
			expect(stored).toBe(10 - priority)
			expect(tasktracePriorityNumber(stored)).toBe(priority)
		}
		expect(tasktracePriorityNumber(10)).toBe(0)
		expect(tasktracePriorityNumber(2)).toBe(8)
	})

	it('does not allow invalid priorities to be written', () => {
		for (const priority of [-1, 10, 1.5, NaN]) expect(() => tasktraceStoredPriority(priority)).toThrow(RangeError)
	})
})
