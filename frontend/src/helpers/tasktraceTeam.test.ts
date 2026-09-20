import {describe, expect, it} from 'vitest'
import {collaborationMembers, overrideTeamLinkRepository, readTeamCommentMarker, serializeTeamCommentMarker, teamCommentAuthor} from './tasktraceTeam'

function marker(value: object) {
	const bytes = new TextEncoder().encode(JSON.stringify(value))
	let binary = ''
	for (const byte of bytes) binary += String.fromCharCode(byte)
	return btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

function decodeTeamLink(link: string) {
	const encoded = link.slice('tasktrace-team://import/'.length)
	const padded = encoded.replace(/-/g, '+').replace(/_/g, '/') + '='.repeat((4 - encoded.length % 4) % 4)
	return JSON.parse(new TextDecoder().decode(Uint8Array.from(atob(padded), char => char.charCodeAt(0))))
}

describe('TaskTrace collaboration members', () => {
	it('includes the owner and current user for legacy member lists', () => {
		expect(collaborationMembers({owner: 'owner', members: ['teammate']}, 'current')).toEqual(['owner', 'current', 'teammate'])
	})

	it('deduplicates usernames without regard to case', () => {
		expect(collaborationMembers({owner: 'Alice', members: ['alice', 'Bob', ' bob ']}, 'ALICE')).toEqual(['Alice', 'Bob'])
	})
})

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

	it('round-trips stable comment identity and author metadata', () => {
		const value = {id: 'shared-comment-id', author: '协作成员'}
		expect(readTeamCommentMarker(serializeTeamCommentMarker(value))).toEqual(value)
	})

	it('replaces an unreachable computer name with an IPv4 teamData path', () => {
		const link = `tasktrace-team://import/${marker({schema: 1, repository: '\\\\TASK-PC\\teamData', repositories: ['\\\\10.0.0.8\\teamData'], share_id: 'share', secret: 'secret'})}`
		const overridden = overrideTeamLinkRepository(link, '\\\\10.143.58.8')

		expect(decodeTeamLink(overridden)).toMatchObject({
			repository: '\\\\10.143.58.8\\teamData',
			repositories: [],
			share_id: 'share',
		})
	})
})
