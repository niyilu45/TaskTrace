import type {TaskTraceTeamConflict} from '@/client/generated'
import {manualCompletion} from './tasktraceCompletion'
import {attachmentImageKey} from './tasktraceImages'

export type ConflictDetail = {key: string, label: string, text: string, images?: string, comparison: string}
export function conflictFieldLabel(field = '') {
	if (field.startsWith('outstanding:')) return '遗留事项'
	return ({percent_done: '填写完成度', title: '任务名', description: '任务描述', done: '完成状态', status: '任务状态', outstanding: '遗留事项清单'} as Record<string, string>)[field] || '内容'
}
const statusLabels: Record<string, string> = {todo: '待办', 'to-do': '待办', doing: '进行中', done: '已完成', hold: '暂停'}

function richDetail(key: string, label: string, html: string): ConflictDetail {
	const doc = new DOMParser().parseFromString(html, 'text/html')
	doc.querySelectorAll('script, style, [data-tasktrace-outstanding-state]').forEach(node => node.remove())
	const images = [...doc.querySelectorAll('img')]
	const imageKeys = images.map(image => attachmentImageKey(image.getAttribute('data-tasktrace-src') || image.getAttribute('data-src') || image.getAttribute('src') || ''))
	const imageHtml = images.map(image => {
		const source = image.getAttribute('src') || ''
		return source.startsWith('tasktrace-team-attachment:') ? '<p>图片尚未同步到本机</p>' : image.outerHTML
	}).join('')
	images.forEach(image => image.remove())
	doc.querySelectorAll('br').forEach(node => node.replaceWith(doc.createTextNode('\n')))
	doc.querySelectorAll('p, div, li, h1, h2, h3, blockquote').forEach(node => node.append(doc.createTextNode('\n')))
	const text = (doc.body.textContent || '').trim()
	return {key, label, text: text || (images.length ? '仅包含图片' : '无'), images: imageHtml, comparison: JSON.stringify([text, imageKeys])}
}
function plain(key: string, label: string, text: string): ConflictDetail { return {key, label, text, comparison: text} }

export function conflictDetails(field = '', value = ''): ConflictDetail[] {
	if (field === 'percent_done') return [plain('completion', '填写完成度', `${manualCompletion(Number(value) * 100)}%`)]
	if (field === 'done') return [plain('done', '完成状态', value === 'true' ? '已完成' : '未完成')]
	if (field === 'status') return [plain('status', '任务状态', statusLabels[value] || value || '未设置')]
	if (field === 'description') return [richDetail('content', '描述', value)]
	if (field === 'outstanding' || field.startsWith('outstanding:')) {
		if (!value) return [plain('presence', '条目', '此版本不保留该遗留事项')]
		const doc = new DOMParser().parseFromString(value, 'text/html')
		const state = doc.querySelector('[data-tasktrace-outstanding-state], li[data-id]')
		const done = state?.getAttribute('data-done') === 'true'
		const reminder = state?.getAttribute('data-reminder') || ''
		const note = doc.querySelector('aside[data-tasktrace-outstanding-note]')
		const noteHtml = note?.innerHTML || ''
		note?.remove()
		doc.querySelectorAll('[data-tasktrace-outstanding-state]').forEach(node => node.remove())
		const details = [plain('presence', '条目', '保留遗留事项'), richDetail('content', '内容', doc.body.innerHTML), plain('done', '完成状态', done ? '已完成' : '未完成'), richDetail('note', '备注', noteHtml)]
		details.push(plain('completion', '填写完成度', `${manualCompletion(state?.getAttribute('data-progress'))}%`))
		details.push(plain('reminder', '提醒时间', reminder ? (Number.isNaN(Date.parse(reminder)) ? reminder : new Date(reminder).toLocaleString()) : '未设置'))
		return details
	}
	return [plain('content', conflictFieldLabel(field), value || '空')]
}

// Bound the work to linear time even for very long notes. Vue escapes every segment.
export function changedText(base: string, next: string) {
	if (base === next) return [{text: next, changed: false}]
	const a = Array.from(base), b = Array.from(next)
	let start = 0, end = 0
	while (start < a.length && start < b.length && a[start] === b[start]) start++
	while (end < a.length - start && end < b.length - start && a[a.length - end - 1] === b[b.length - end - 1]) end++
	return [
		{text: b.slice(0, start).join(''), changed: false},
		{text: b.slice(start, b.length - end).join(''), changed: true},
		{text: end ? b.slice(b.length - end).join('') : '', changed: false},
	].filter(part => part.text)
}

export function conflictVersions(conflict: TaskTraceTeamConflict) {
	return [
		{key: 'base', author: '', label: '上次同步版本', value: conflict.base || '', preview: conflict.base_preview ?? conflict.base ?? ''},
		...(conflict.options || []).map((option, index) => ({key: `option-${index}`, label: '成员修改版本', author: option.author || '', value: option.value || '', preview: option.preview ?? option.value ?? ''})),
	].map(version => ({...version, details: conflictDetails(conflict.field, version.preview)}))
}
