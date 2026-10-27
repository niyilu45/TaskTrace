export interface TeamAccessConsent {
	elevate: boolean
	createShare: boolean
}

export class TeamAccessCancelled extends Error {
	constructor() {
		super('已取消权限修改，协作成员和任务权限未保存。')
		this.name = 'TeamAccessCancelled'
	}
}

export function isTeamAccessCancelled(cause: unknown) {
	return cause instanceof TeamAccessCancelled
}

function accessErrorText(cause: unknown) {
	const value = cause as {reason?: {response?: {data?: unknown}}, response?: {data?: unknown}} | null
	const data = (value?.reason?.response?.data ?? value?.response?.data ?? cause) as {
		message?: string, detail?: string, errors?: {message?: string}[]
	} | null
	return [data?.detail, data?.message, ...(data?.errors ?? []).map(item => item.message)]
		.filter(Boolean).join(' ').toLocaleLowerCase()
}

const shareMarkers = ['windows file sharing confirmation required', 'teamdata 尚未创建 windows 文件共享']
const administratorMarkers = ['windows administrator authorization required', '需要 windows 管理员授权', 'access is denied', 'access denied', '拒绝访问', '系统错误 5', 'sesecurityprivilege']

export function needsTeamAccessConsent(cause: unknown) {
	const message = accessErrorText(cause)
	return [...shareMarkers, ...administratorMarkers].some(marker => message.includes(marker))
}

export function teamAccessHeaders(consent: TeamAccessConsent) {
	return {
		...(consent.elevate ? {'X-TaskTrace-Elevate': true as const} : {}),
		...(consent.createShare ? {'X-TaskTrace-Create-Share': true as const} : {}),
	}
}

// Consent is scoped to this action, never remembered as a blanket permission.
export async function withTeamAccessConsent<T>(
	action: (consent: TeamAccessConsent) => Promise<T>,
	label: string,
	confirm: (message: string) => boolean = message => window.confirm(message),
): Promise<T> {
	const consent: TeamAccessConsent = {elevate: false, createShare: false}
	for (;;) {
		try {
			return await action({...consent})
		} catch (cause) {
			const message = accessErrorText(cause)
			if (!consent.createShare && shareMarkers.some(marker => message.includes(marker))) {
				if (!confirm('teamData 尚未共享，其他电脑无法访问协作数据。是否将本机 teamData 设为 Windows 共享文件夹并继续？\n\n仅为本机用户和选定协作成员设置权限，不向所有人开放。')) throw new TeamAccessCancelled()
				consent.createShare = true
				continue
			}
			if (!consent.elevate && administratorMarkers.some(marker => message.includes(marker))) {
				if (!confirm(`${label}需要 Windows 管理员授权。是否继续？\n\nWindows 将弹出用户账户控制窗口，仅为本次共享权限操作短暂提升权限，TaskTrace 仍以普通权限运行。`)) throw new TeamAccessCancelled()
				consent.elevate = true
				continue
			}
			throw cause
		}
	}
}
