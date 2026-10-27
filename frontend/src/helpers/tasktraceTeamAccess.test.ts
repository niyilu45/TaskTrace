import {describe, expect, it, vi} from 'vitest'
import {isTeamAccessCancelled, teamAccessHeaders, withTeamAccessConsent} from './tasktraceTeamAccess'

const shareRequired = {status: 409, detail: 'Windows file sharing confirmation required', errors: [{message: 'teamData 尚未创建 Windows 文件共享'}]}
const adminRequired = {status: 403, detail: 'Windows administrator authorization required'}

describe('teamData access consent', () => {
	it('asks before sharing, then asks for administrator consent only if required', async () => {
		const action = vi.fn().mockRejectedValueOnce(shareRequired).mockRejectedValueOnce(adminRequired).mockResolvedValue('saved')
		const confirm = vi.fn((_message: string) => true)
		expect(await withTeamAccessConsent(action, '添加成员', confirm)).toBe('saved')
		expect(action.mock.calls.map(([options]) => options)).toEqual([
			{createShare: false, elevate: false},
			{createShare: true, elevate: false},
			{createShare: true, elevate: true},
		])
		expect(confirm).toHaveBeenCalledTimes(2)
		expect(confirm.mock.calls[0][0]).toContain('是否将本机 teamData')
		expect(confirm.mock.calls[1][0]).toContain('管理员授权')
	})

	it.each([shareRequired, adminRequired])('cancels without retrying when consent is declined', async cause => {
		const action = vi.fn().mockRejectedValue(cause)
		const confirm = vi.fn(() => false)
		await expect(withTeamAccessConsent(action, '添加成员', confirm)).rejects.toSatisfy(isTeamAccessCancelled)
		expect(action).toHaveBeenCalledTimes(1)
	})

	it('does not request administrator access for an ordinary task permission denial', async () => {
		const cause = {status: 403, detail: 'forbidden'}
		const confirm = vi.fn()
		await expect(withTeamAccessConsent(vi.fn().mockRejectedValue(cause), '修改任务', confirm)).rejects.toBe(cause)
		expect(confirm).not.toHaveBeenCalled()
	})

	it('does not repeatedly prompt when a confirmed operation still fails', async () => {
		const action = vi.fn().mockRejectedValue(adminRequired)
		const confirm = vi.fn((_message: string) => true)
		await expect(withTeamAccessConsent(action, '添加成员', confirm)).rejects.toBe(adminRequired)
		expect(action).toHaveBeenCalledTimes(2)
		expect(confirm).toHaveBeenCalledTimes(1)
	})

	it('recognizes nested API errors and keeps consent limited to one action', async () => {
		const action = vi.fn().mockRejectedValueOnce({response: {data: shareRequired}}).mockResolvedValue('saved')
		await withTeamAccessConsent(action, '添加成员', () => true)
		await withTeamAccessConsent(action, '添加其他成员', () => true)
		expect(action.mock.calls[2][0]).toEqual({createShare: false, elevate: false})
		expect(teamAccessHeaders({createShare: true, elevate: true})).toEqual({'X-TaskTrace-Create-Share': true, 'X-TaskTrace-Elevate': true})
		expect(teamAccessHeaders({createShare: false, elevate: false})).toEqual({})
	})
})
