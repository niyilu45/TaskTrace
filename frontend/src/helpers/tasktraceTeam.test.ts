import {describe, expect, it} from 'vitest'
import {readTeamCommentMarker, teamCommentAuthor} from './tasktraceTeam'

function marker(value: object) {
	const bytes = new TextEncoder().encode(JSON.stringify(value))
	let binary = ''
	for (const byte of bytes) binary += String.fromCharCode(byte)
	return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

describe('TaskTrace team comment metadata', () => {
	it('shows the original LAN author on imported comments', () => {
		const html = `<p>进展</p><!--tasktrace-team:${marker({id: 'one', author: 'alice'})}-->`
		expect(readTeamCommentMarker(html)).toEqual({id: 'one', author: 'alice'})
		expect(teamCommentAuthor(html, 'local-user')).toBe('alice')
	})

	it('falls back to the local comment author for personal tasks', () => {
		expect(teamCommentAuthor('<p>进展</p>', 'local-user')).toBe('local-user')
		expect(readTeamCommentMarker('broken')).toBeNull()
	})
})
