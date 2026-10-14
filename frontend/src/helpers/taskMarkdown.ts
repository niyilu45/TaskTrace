import {tasksTasktraceMarkdown} from '@/client/generated'

export function markdownFilename(title: string): string {
	// Windows forbids control characters in filenames.
	// eslint-disable-next-line no-control-regex
	let name = title.replace(/[<>:"/\\|?*\u0000-\u001f]/g, '_').replace(/[ .]+$/g, '').trim().slice(0, 100) || '任务'
	if (/^(CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])(?:\.|$)/i.test(name)) name = `_${name}`
	return `${name}.md`
}

interface MarkdownFileHandle {createWritable(): Promise<{write(data: Blob): Promise<void>, close(): Promise<void>, abort(): Promise<void>}>}
type PickerWindow = Window & {showSaveFilePicker?: (options: {suggestedName: string, types: {description: string, accept: Record<string, string[]>}[]}) => Promise<MarkdownFileHandle>}

export async function saveTaskMarkdown(taskId: number, title: string) {
	const picker = (window as PickerWindow).showSaveFilePicker
	// Open the picker during the user's gesture, before the network request.
	const handle = picker ? await picker.call(window, {suggestedName: markdownFilename(title), types: [{description: 'Markdown 文档', accept: {'text/markdown': ['.md']}}]}) : null
	const {data} = await tasksTasktraceMarkdown({throwOnError: true, path: {task: taskId}})
	const blob = new Blob([data.markdown || ''], {type: 'text/markdown;charset=utf-8'})
	if (handle) {
		const writable = await handle.createWritable()
		try {await writable.write(blob); await writable.close()}
		catch (error) {await writable.abort(); throw error}
	} else {
		const href = URL.createObjectURL(blob)
		const anchor = document.createElement('a')
		anchor.href = href; anchor.download = data.filename || markdownFilename(title)
		anchor.click()
		setTimeout(() => URL.revokeObjectURL(href), 30_000)
	}
	return data.warnings || []
}
