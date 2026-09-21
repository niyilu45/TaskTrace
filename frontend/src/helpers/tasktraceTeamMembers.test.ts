import {describe, expect, it} from 'vitest'

import {
	mergeTeamMemberCandidates,
	teamMemberCandidatePayload,
	teamMemberDisplayName,
	teamMemberKey,
	teamMemberVerification,
} from './tasktraceTeamMembers'

describe('TaskTrace team member identity', () => {
	it('normalizes domain, employee id, and email forms to one member', () => {
		expect(teamMemberKey('CHINA\\012345')).toBe('012345')
		expect(teamMemberKey('012345@company.example')).toBe('012345')
	})

	it('merges a quick account result with richer directory information', () => {
		const [member] = mergeTeamMemberCandidates(
			[{username: '012345', account_name: 'CHINA\\012345'}],
			[{username: '012345', account_name: 'CHINA\\012345', display_name: '张三', email: '012345@company.example'}],
		)

		expect(member).toEqual({
			username: '012345',
			account_name: 'CHINA\\012345',
			display_name: '张三',
			email: '012345@company.example',
		})
		expect(teamMemberDisplayName(member)).toBe('张三')
		expect(teamMemberVerification(member)).toBe('姓名：张三；工号：012345；邮箱：012345@company.example')
	})

	it('keeps one entry when already-added members use different account forms', () => {
		const members = mergeTeamMemberCandidates(
			[{username: '012345', account_name: 'CHINA\\012345'}],
			[{username: '012345', account_name: '012345'}],
			[{username: '012345', account_name: '012345@company.example'}],
		)

		expect(members).toHaveLength(1)
	})

	it('creates a team member payload from the Windows identity', () => {
		expect(teamMemberCandidatePayload({
			username: '654321',
			account_name: 'CHINA\\654321',
			display_name: '张三',
			email: 'zhangsan@example.com',
		})).toEqual({
			username: '654321',
			accountName: 'CHINA\\654321',
			name: '张三',
			email: 'zhangsan@example.com',
		})
	})
})
