import {apiV2Url, AuthenticatedHTTPFactory} from '@/helpers/fetcher'

export type TaskTraceDraftKind = 'progress' | 'outstanding'

function path(kind: TaskTraceDraftKind, taskId: number, key: string) {
	return apiV2Url(`tasktrace/drafts/${kind}/${taskId}/${encodeURIComponent(key)}`)
}

export async function readTaskTraceDraft<T>(kind: TaskTraceDraftKind, taskId: number, key: string): Promise<T | null> {
	const {data} = await AuthenticatedHTTPFactory().get(path(kind, taskId, key))
	if (!data?.exists || typeof data.content !== 'string') return null
	return JSON.parse(data.content) as T
}

export async function writeTaskTraceDraft(kind: TaskTraceDraftKind, taskId: number, key: string, value: unknown) {
	await AuthenticatedHTTPFactory().put(path(kind, taskId, key), {content: JSON.stringify(value)})
}

export async function deleteTaskTraceDraft(kind: TaskTraceDraftKind, taskId: number, key: string) {
	await AuthenticatedHTTPFactory().delete(path(kind, taskId, key))
}

export function fileAsDataUrl(file: File): Promise<string> {
	return new Promise((resolve, reject) => {
		const reader = new FileReader()
		reader.onload = () => resolve(String(reader.result || ''))
		reader.onerror = () => reject(reader.error || new Error('File read failed'))
		reader.readAsDataURL(file)
	})
}

export async function dataUrlAsFile(value: string, name = 'cached-image.png', type = 'image/png') {
	const blob = await (await fetch(value)).blob()
	return new File([blob], name, {type: blob.type || type})
}
