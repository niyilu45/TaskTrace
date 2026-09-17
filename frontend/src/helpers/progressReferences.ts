export type ProgressReference = {
	id: string
	taskId: number
	date: string
	commentIds: number[]
	html: string
}

type ReferenceDay = {id?: number, mergedIds: number[], html: string}
const REFERENCE_SELECTOR = 'blockquote[data-tasktrace-reference]'
const BLOCK_ELEMENTS = new Set(['P', 'DIV', 'LI', 'UL', 'OL', 'BLOCKQUOTE', 'PRE', 'H1', 'H2', 'H3', 'H4', 'H5', 'H6', 'TABLE', 'TR', 'TD', 'TH'])
const positiveId = (value: unknown): value is number => typeof value === 'number' && Number.isSafeInteger(value) && value > 0
function validDate(value: unknown): value is string {
	if (typeof value !== 'string' || !/^\d{4}-\d{2}-\d{2}$/.test(value)) return false
	const date = new Date(`${value}T00:00:00Z`)
	return !Number.isNaN(+date) && date.toISOString().slice(0, 10) === value
}
function validIds(value: unknown): number[] | null {
	if (!Array.isArray(value) || !value.length || !Array.from(value).every(positiveId)) return null
	return [...new Set(value)].sort((a, b) => a - b)
}
function metadata(value: unknown): Omit<ProgressReference, 'html'> | null {
	if (!value || typeof value !== 'object') return null
	const candidate = value as Partial<ProgressReference>
	if (typeof candidate.id !== 'string' || !/^r[a-f0-9]{32}$/.test(candidate.id) || !positiveId(candidate.taskId) || !validDate(candidate.date)) return null
	const commentIds = validIds(candidate.commentIds)
	return commentIds ? {id: candidate.id, taskId: candidate.taskId, date: candidate.date, commentIds} : null
}
function wrapper(element: Element) {
	if (element.tagName !== 'BLOCKQUOTE' || element.getAttribute('data-tasktrace-reference') !== '1') return null
	const taskId = element.getAttribute('data-task-id') || ''
	const ids = element.getAttribute('data-comment-ids') || ''
	if (!/^[1-9]\d*$/.test(taskId) || !/^[1-9]\d*(?:,[1-9]\d*)*$/.test(ids)) return null
	const data = metadata({id: element.getAttribute('data-reference-id'), taskId: Number(taskId), date: element.getAttribute('data-date'), commentIds: ids.split(',').map(Number)})
	const children = Array.from(element.children)
	if (children.length !== 2 || children[0].tagName !== 'P' || Array.from(element.childNodes).some(node => node.nodeType === Node.TEXT_NODE && node.textContent?.trim())) return null
	const content = children.filter(child => child.tagName === 'DIV' && child.getAttribute('data-tasktrace-reference-content') === '1')
	return data && content.length === 1 ? {data, content: content[0]} : null
}
function escapeText(value: string) {
	return value.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;')
}
function snapshotHtml(value: string, taskId: number) {
	const doc = document.createElement('template')
	doc.innerHTML = value
	// Prior snapshots never become part of a new snapshot, even if nested in rich text.
	for (const element of doc.content.querySelectorAll(REFERENCE_SELECTOR)) if (wrapper(element)) element.remove()
	doc.content.querySelectorAll('script,style,iframe,object,embed,svg,math,template,noscript,input,textarea,select,button,link,meta').forEach(element => element.remove())
	const output: string[] = []
	let text = ''
	const flush = () => {
		const plain = text.replace(/\r\n?/g, '\n').split('\n').map(line => line.trim()).join('\n').replace(/\n{3,}/g, '\n\n').trim()
		if (plain) output.push(`<p>${escapeText(plain).replace(/\n/g, '<br>')}</p>`)
		text = ''
	}
	const walk = (node: Node) => {
		if (node.nodeType === Node.TEXT_NODE) { text += node.textContent || ''; return }
		if (!(node instanceof Element)) return
		if (node.tagName === 'IMG') {
			flush()
			const match = (node.getAttribute('data-src') || node.getAttribute('src') || '').match(/^\/api\/v([12])\/tasks\/([1-9]\d*)\/attachments\/([1-9]\d*)$/)
			if (match && Number(match[2]) === taskId && positiveId(Number(match[3]))) output.push(`<p><img src="/api/v${match[1]}/tasks/${taskId}/attachments/${Number(match[3])}"></p>`)
			return
		}
		if (node.tagName === 'BR') { text += '\n'; return }
		if (BLOCK_ELEMENTS.has(node.tagName)) text += '\n'
		node.childNodes.forEach(walk)
		if (BLOCK_ELEMENTS.has(node.tagName)) text += '\n'
	}
	doc.content.childNodes.forEach(walk)
	flush()
	return output.join('')
}

export function normalizeProgressReferences(value: unknown): ProgressReference[] {
	if (!Array.isArray(value)) return []
	const result: ProgressReference[] = []
	const seen = new Set<string>()
	for (const candidate of value) {
		const data = metadata(candidate)
		if (!data || typeof candidate.html !== 'string' || seen.has(data.id)) continue
		seen.add(data.id)
		result.push({...data, html: snapshotHtml(candidate.html, data.taskId)})
	}
	return result
}

export function splitProgressReferences(html: string): {html: string, references: ProgressReference[]} {
	const doc = document.createElement('template')
	doc.innerHTML = html
	const references: ProgressReference[] = []
	for (const element of doc.content.querySelectorAll(REFERENCE_SELECTOR)) {
		// Unknown outer wrappers remain ordinary content, including their descendants.
		if (element.parentElement?.closest(REFERENCE_SELECTOR)) continue
		const parsed = wrapper(element)
		if (!parsed) continue
		references.push({...parsed.data, html: parsed.content.innerHTML})
		element.remove()
	}
	return {html: doc.innerHTML, references: normalizeProgressReferences(references)}
}

export function serializeProgressReferences(refs: ProgressReference[]): string {
	return normalizeProgressReferences(refs).map(ref => `<blockquote data-tasktrace-reference="1" data-reference-id="${ref.id}" data-task-id="${ref.taskId}" data-date="${ref.date}" data-comment-ids="${ref.commentIds.join(',')}"><p><strong>引用 ${ref.date} 的进展</strong> <a href="/tasks/${ref.taskId}#comment-${ref.commentIds[ref.commentIds.length - 1]}">查看原记录</a></p><div data-tasktrace-reference-content="1">${ref.html}</div></blockquote>`).join('')
}

export function createProgressReference(taskId: number, sourceDate: string, currentDate: string, day: ReferenceDay): ProgressReference | null {
	if (!positiveId(taskId) || !validDate(sourceDate) || !validDate(currentDate) || sourceDate >= currentDate || !day || typeof day.html !== 'string' || !Array.isArray(day.mergedIds)) return null
	if (day.id !== undefined && !positiveId(day.id)) return null
	const commentIds = validIds([...day.mergedIds, ...(day.id === undefined ? [] : [day.id])])
	if (!commentIds) return null
	const html = snapshotHtml(day.html, taskId)
	if (!html) return null
	const bytes = crypto.getRandomValues(new Uint8Array(16))
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	const id = 'r' + Array.from(bytes, byte => byte.toString(16).padStart(2, '0')).join('')
	return {id, taskId, date: sourceDate, commentIds, html}
}
