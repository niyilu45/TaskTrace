import type {TaskTraceTeamMemberCandidate} from '@/client/generated'

export function teamMemberKey(value = '') {
	return (value.trim().split('\\').pop()?.split('@')[0] || '').toLocaleLowerCase()
}

export function teamMemberEmployeeId(candidate: TaskTraceTeamMemberCandidate | string) {
	const value = typeof candidate === 'string'
		? candidate
		: candidate.username || candidate.account_name || candidate.email || ''
	return value.trim().split('\\').pop()?.split('@')[0] || ''
}

export function teamMemberDisplayName(candidate: TaskTraceTeamMemberCandidate | string) {
	if (typeof candidate === 'string') return teamMemberEmployeeId(candidate)
	const name = candidate.display_name?.trim()
	return name && teamMemberKey(name) !== teamMemberKey(candidate.account_name || candidate.username)
		? name
		: teamMemberEmployeeId(candidate)
}

export function teamMemberVerification(candidate: TaskTraceTeamMemberCandidate | string) {
	const value = typeof candidate === 'string' ? {username: candidate} : candidate
	return [
		`姓名：${teamMemberDisplayName(value) || '未提供'}`,
		`工号：${teamMemberEmployeeId(value) || '未提供'}`,
		`邮箱：${value.email?.trim() || '未提供'}`,
	].join('；')
}

export function mergeTeamMemberCandidates(...groups: TaskTraceTeamMemberCandidate[][]) {
	const merged = new Map<string, TaskTraceTeamMemberCandidate>()
	for (const candidate of groups.flat()) {
		const key = teamMemberKey(candidate.account_name || candidate.username || candidate.email)
		if (!key) continue
		const current = merged.get(key)
		merged.set(key, {
			username: candidate.username || current?.username || key,
			account_name: candidate.account_name || current?.account_name || candidate.username,
			display_name: candidate.display_name || current?.display_name,
			email: candidate.email || current?.email,
		})
	}
	return [...merged.values()]
}

export function teamMemberCandidatePayload(candidate: TaskTraceTeamMemberCandidate) {
	return {
		username: teamMemberEmployeeId(candidate),
		accountName: candidate.account_name || candidate.username || '',
		name: teamMemberDisplayName(candidate),
		email: candidate.email || '',
	}
}
