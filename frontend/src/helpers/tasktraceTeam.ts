const marker = /<!--tasktrace-team:([A-Za-z0-9_-]+)-->/
const teamLinkPrefix = 'tasktrace-team://import/'

export type TeamCommentMarker = {id: string, author: string}

export function readTeamCommentMarker(html: string): TeamCommentMarker | null {
	const encoded = html.match(marker)?.[1]
	if (!encoded) return null
	try {
		const padded = encoded.replace(/-/g, '+').replace(/_/g, '/') + '='.repeat((4 - encoded.length % 4) % 4)
		const bytes = Uint8Array.from(atob(padded), char => char.charCodeAt(0))
		const value = JSON.parse(new TextDecoder().decode(bytes)) as TeamCommentMarker
		return value?.id ? value : null
	} catch {
		return null
	}
}

export function teamCommentAuthor(html: string, fallback: string) {
	return readTeamCommentMarker(html)?.author || fallback
}

export function serializeTeamCommentMarker(value: TeamCommentMarker) {
	const bytes = new TextEncoder().encode(JSON.stringify(value))
	let binary = ''
	for (const byte of bytes) binary += String.fromCharCode(byte)
	return `<!--tasktrace-team:${btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')}-->`
}

export function createTeamCommentId() {
	return globalThis.crypto?.randomUUID?.() || `web-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`
}

export function overrideTeamLinkRepository(link: string, repository: string) {
	let normalized = repository.trim().replace(/^['"]|['"]$/g, '').replace(/[\\/]+$/, '')
	if (!normalized) return link
	if (/^\\\\[^\\/]+$/.test(normalized)) normalized += '\\teamData'
	const trimmed = link.trim()
	if (!trimmed.toLowerCase().startsWith(teamLinkPrefix)) return link
	try {
		const encoded = trimmed.slice(teamLinkPrefix.length)
		const padded = encoded.replace(/-/g, '+').replace(/_/g, '/') + '='.repeat((4 - encoded.length % 4) % 4)
		const bytes = Uint8Array.from(atob(padded), char => char.charCodeAt(0))
		const value = JSON.parse(new TextDecoder().decode(bytes)) as {repository?: string, repositories?: string[]}
		value.repository = normalized
		value.repositories = []
		const result = new TextEncoder().encode(JSON.stringify(value))
		let binary = ''
		for (const byte of result) binary += String.fromCharCode(byte)
		return teamLinkPrefix + btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
	} catch {
		return link
	}
}

export function collaborationMembers(
	binding?: {owner?: string, members?: Array<string> | null},
	currentUsername = '',
) {
	const seen = new Set<string>()
	const result: string[] = []
	for (const candidate of [binding?.owner, currentUsername, ...(binding?.members ?? [])]) {
		const member = candidate?.trim()
		if (!member) continue
		const key = (member.split('\\').pop()?.split('@')[0] || member).toLocaleLowerCase()
		if (seen.has(key)) continue
		seen.add(key)
		result.push(member)
	}
	return result
}
