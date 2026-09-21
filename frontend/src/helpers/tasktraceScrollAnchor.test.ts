import {describe, expect, it} from 'vitest'
import {captureTasktraceScrollAnchor, tasktraceScrollAnchorDelta} from './tasktraceScrollAnchor'

function anchor(key: string, top: number, bottom = top + 40) {
	const element = document.createElement('div')
	element.dataset.progressAnchor = key
	element.getBoundingClientRect = () => ({top, bottom, left: 0, right: 100, width: 100, height: bottom - top, x: 0, y: top, toJSON: () => ({})})
	return element
}

describe('TaskTrace display position anchor', () => {
	it('keeps the first visible task at the same viewport position', () => {
		const root = document.createElement('div')
		root.append(anchor('task-1', -80), anchor('task-2', 24), anchor('task-3', 80))
		const saved = captureTasktraceScrollAnchor(root, 500, 600)!
		expect(saved.keys[0]).toBe('task-2')
		root.replaceChildren(anchor('task-new', -120), anchor('task-2', 64), anchor('task-3', 120))
		expect(tasktraceScrollAnchorDelta(root, saved)).toBe(40)
	})

	it('falls forward to the next task when the visible task was deleted', () => {
		const root = document.createElement('div')
		root.append(anchor('task-1', -80), anchor('task-2', 24), anchor('task-3', 80))
		const saved = captureTasktraceScrollAnchor(root, 500, 600)!
		root.replaceChildren(anchor('task-1', -80), anchor('task-3', 24))
		expect(tasktraceScrollAnchorDelta(root, saved)).toBe(0)
	})
})
