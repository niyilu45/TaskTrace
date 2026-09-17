import {undoGroupHeaders} from '@/helpers/tasktraceUndo'
import {taskCommentsList, taskCommentsCreate, taskCommentsUpdate, type TaskComment} from '@/client/generated'
import {sortProgressNotes} from './progressNotes'
export const sharedHeading = 'TaskTrace 遗留事项清单'
export type OutstandingItem = {id: string, html: string}
export function sharedOutstanding(history: TaskComment[]) {
	const record = [...history].sort((a, b) => (b.id || 0) - (a.id || 0)).find(note => new DOMParser().parseFromString(note.comment || '', 'text/html').querySelector('h3')?.textContent === sharedHeading)
	if (record) {
		const doc = new DOMParser().parseFromString(record.comment || '', 'text/html')
		return {id: record.id, items: Array.from(doc.querySelectorAll('ul > li')).map((li, index) => ({id: li.getAttribute('data-id') || `item-${index}`, html: li.innerHTML}))}
	}
	const latest = sortProgressNotes(history).find(note => note.daily)
	const html = latest?.outstanding || ''
	return {id: undefined, items: html ? html.split(/<br\s*\/?>(?:\r?\n)?/i).filter(value => value.trim()).map((html, index) => ({id: `legacy-${latest?.id}-${index}`, html})) : []}
}
export function outstandingHtml(history: TaskComment[]) {
	return sharedOutstanding(history).items.map(item => `<p>${item.html}</p>`).join('')
}
export async function readTaskHistory(taskId: number) {
	const history: TaskComment[] = []
	for (let page = 1; ; page++) {
		const result = await taskCommentsList({path: {task: taskId}, query: {page, per_page: 100, order_by: 'desc'}})
		const items = result.data.items || []
		history.push(...items)
		if (page >= (result.data.total_pages || 1) || !items.length) break
	}
	return history
}
export async function changeOutstanding(taskId: number, change: (items: OutstandingItem[]) => OutstandingItem[], headers = undoGroupHeaders()) {
	const current = sharedOutstanding(await readTaskHistory(taskId))
	const items = change(current.items)
	const comment = `<h3>${sharedHeading}</h3><ul>${items.map(item => `<li data-id="${item.id.replace(/[^a-zA-Z0-9-]/g, '')}">${item.html}</li>`).join('')}</ul>`
	if (current.id) await taskCommentsUpdate({path: {task: taskId, commentid: current.id}, body: {comment}, headers})
	else await taskCommentsCreate({path: {task: taskId}, body: {comment}, headers})
	return items
}
