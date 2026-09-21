export type TasktraceScrollAnchor = {
	keys: string[]
	top: number
	scrollY: number
}

export function captureTasktraceScrollAnchor(root: HTMLElement | undefined, scrollY: number, viewportHeight: number): TasktraceScrollAnchor | null {
	if (!root) return null
	const elements = [...root.querySelectorAll<HTMLElement>('[data-progress-anchor]')]
	if (!elements.length) return null
	const index = Math.max(0, elements.findIndex(element => {
		const rect = element.getBoundingClientRect()
		return rect.bottom > 0 && rect.top < viewportHeight
	}))
	const current = elements[index]
	const key = current.dataset.progressAnchor
	if (!key) return null
	const following = elements.slice(index + 1).map(element => element.dataset.progressAnchor).filter((value): value is string => !!value)
	const preceding = elements.slice(0, index).reverse().map(element => element.dataset.progressAnchor).filter((value): value is string => !!value)
	return {keys: [key, ...following, ...preceding], top: current.getBoundingClientRect().top, scrollY}
}

export function tasktraceScrollAnchorDelta(root: HTMLElement | undefined, anchor: TasktraceScrollAnchor): number | null {
	if (!root) return null
	const elements = [...root.querySelectorAll<HTMLElement>('[data-progress-anchor]')]
	for (const key of anchor.keys) {
		const element = elements.find(candidate => candidate.dataset.progressAnchor === key)
		if (element) return element.getBoundingClientRect().top - anchor.top
	}
	return null
}
