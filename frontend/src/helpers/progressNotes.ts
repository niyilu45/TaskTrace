import type {TaskComment} from '@/client/generated'

export function parseProgressNote(note: TaskComment) {
	// Parse in an inert document; all returned HTML is sanitized by ReadonlyRichText before rendering.
	const doc = new DOMParser().parseFromString(note.comment || '', 'text/html')
	const heading = doc.body.querySelector('h3')
	const match = heading?.textContent?.trim().match(/^每日进展\s*[·:：]\s*(\d{4}-\d{2}-\d{2})$/)
	const created = new Date(note.created || '')
	const fallback = Number.isNaN(+created) ? '日期未知' : `${created.getFullYear()}-${String(created.getMonth() + 1).padStart(2, '0')}-${String(created.getDate()).padStart(2, '0')}`
	const candidate = match?.[1]
	const parsedDate = candidate ? new Date(candidate + 'T00:00:00Z') : null
	const validDate = parsedDate && !Number.isNaN(+parsedDate) && parsedDate.toISOString().slice(0, 10) === candidate
	const date = validDate ? candidate! : fallback
	let outstanding = ''
	if (match) {
		heading?.remove()
		const label = Array.from(doc.body.querySelectorAll('p')).find(p => p.textContent?.trim() === '遗留问题 / 下一步' && p.querySelector('strong'))
		if (label) {
			const content = label.nextElementSibling
			if (content?.tagName === 'P') { outstanding = content.innerHTML; content.remove() }
			label.remove()
		}
	}
	return {id: note.id, date, daily: !!match, progress: doc.body.innerHTML, outstanding, created: Number.isNaN(+created) ? 0 : +created}
}

export function sortProgressNotes(notes: TaskComment[]) {
	const absorbed = new Set<number>()
	for (const note of notes) {
		const heading = new DOMParser().parseFromString(note.comment || '', 'text/html').querySelector('h3[data-tasktrace-merged]')
		for (const id of (heading?.getAttribute('data-tasktrace-merged') || '').split(',')) { if (Number(id) > 0 && Number(id) !== note.id) absorbed.add(Number(id)) }
	}
	return notes.filter(note => !absorbed.has(note.id || 0)).filter(note => new DOMParser().parseFromString(note.comment || '', 'text/html').querySelector('h3')?.textContent !== 'TaskTrace 遗留事项清单').map(parseProgressNote).sort((a, b) => (b.date === '日期未知' ? '' : b.date).localeCompare(a.date === '日期未知' ? '' : a.date) || b.created - a.created || (b.id || 0) - (a.id || 0))
}

export function mergedDay(history: TaskComment[], date: string) {
	const notes = sortProgressNotes(history).filter(note => note.daily && note.date === date).sort((a, b) => a.created - b.created || (a.id || 0) - (b.id || 0))
	const primary = notes.reduce<number | undefined>((id, note) => Math.max(id || 0, note.id || 0) || undefined, undefined)
	const ids = new Set(notes.map(note => note.id).filter((id): id is number => !!id && id !== primary))
	for (const note of history.filter(note => notes.some(active => active.id === note.id))) {
		const heading = new DOMParser().parseFromString(note.comment || '', 'text/html').querySelector('h3')
		for (const id of (heading?.getAttribute('data-tasktrace-merged') || '').split(',')) if (Number(id) > 0 && Number(id) !== primary) ids.add(Number(id))
	}
	const html = notes.map(note => note.progress).join('')
	const doc = new DOMParser().parseFromString(html, 'text/html')
	const images = Array.from(doc.querySelectorAll('img')).map(img => img.outerHTML).join('')
	doc.querySelectorAll('img').forEach(img => img.remove())
	doc.querySelectorAll('br').forEach(br => br.replaceWith('\n'))
	doc.querySelectorAll('p,div,li').forEach(el => el.append('\n'))
	return {id: primary, mergedIds: [...ids], html, images, text: (doc.body.textContent || '').trim()}
}
