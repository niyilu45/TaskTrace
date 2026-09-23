import {apiV2Url, AuthenticatedHTTPFactory} from '@/helpers/fetcher'

export type TaskTraceDraftKind = 'progress' | 'outstanding'

type EmergencyDraft<T> = {updated: number, value: T}

function emergencyKey(kind: TaskTraceDraftKind, taskId: number, key: string) {
	return `tasktrace-emergency-draft:${kind}:${taskId}:${key}`
}

function readEmergencyDraft<T>(kind: TaskTraceDraftKind, taskId: number, key: string): EmergencyDraft<T> | null {
	try {
		const value = JSON.parse(localStorage.getItem(emergencyKey(kind, taskId, key)) || 'null')
		return Number.isFinite(value?.updated) && value && 'value' in value ? value as EmergencyDraft<T> : null
	} catch {
		return null
	}
}

export function rememberTaskTraceDraft(kind: TaskTraceDraftKind, taskId: number, key: string, value: unknown) {
	try {
		localStorage.setItem(emergencyKey(kind, taskId, key), JSON.stringify({updated: Date.now(), value}))
	} catch { /* The regular .cache writer remains available when browser storage is full. */ }
}

export function forgetRememberedTaskTraceDraft(kind: TaskTraceDraftKind, taskId: number, key: string) {
	try { localStorage.removeItem(emergencyKey(kind, taskId, key)) } catch { /* Ignore unavailable browser storage. */ }
}

function path(kind: TaskTraceDraftKind, taskId: number, key: string) {
	return apiV2Url(`tasktrace/drafts/${kind}/${taskId}/${encodeURIComponent(key)}`)
}

export async function readTaskTraceDraft<T>(kind: TaskTraceDraftKind, taskId: number, key: string): Promise<T | null> {
	const emergency = readEmergencyDraft<T>(kind, taskId, key)
	try {
		const {data} = await AuthenticatedHTTPFactory().get(path(kind, taskId, key))
		if (!data?.exists || typeof data.content !== 'string') return emergency?.value ?? null
		const remoteUpdated = Date.parse(data.updated || '')
		if (emergency && (!Number.isFinite(remoteUpdated) || emergency.updated >= remoteUpdated)) return emergency.value
		return JSON.parse(data.content) as T
	} catch (error) {
		if (emergency) return emergency.value
		throw error
	}
}

export async function writeTaskTraceDraft(kind: TaskTraceDraftKind, taskId: number, key: string, value: unknown) {
	rememberTaskTraceDraft(kind, taskId, key, value)
	await AuthenticatedHTTPFactory().put(path(kind, taskId, key), {content: JSON.stringify(value)})
}

export async function deleteTaskTraceDraft(kind: TaskTraceDraftKind, taskId: number, key: string) {
	forgetRememberedTaskTraceDraft(kind, taskId, key)
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
