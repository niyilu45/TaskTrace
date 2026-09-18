const marker = /<!--tasktrace-team:([A-Za-z0-9_-]+)-->/

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
