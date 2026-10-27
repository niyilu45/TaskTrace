import {i18n} from '@/i18n'
import {notify} from '@kyvg/vue3-notification'

export function getErrorText(r): string {
	const data = r?.reason?.response?.data || r?.response?.data || r
	const status = r?.reason?.response?.status ?? r?.response?.status ?? data?.status
	const detailsText = [data?.detail, data?.message, ...(Array.isArray(data?.errors) ? data.errors.map((item: {message?: string} | null) => item?.message) : [])].filter(Boolean).join(' ')
	const needsAdministrator = /windows administrator authorization required|需要.*管理员授权/i.test(detailsText)
	if (status === 403 && !needsAdministrator) {
		return '操作失败：当前账户没有相应的读写权限。请联系任务所属人检查任务或遗留事项的权限；权限更新后可直接重试。'
	}
	if (!needsAdministrator && /access is denied|access denied|permission denied|拒绝访问|系统错误 5/i.test(detailsText)) {
		return `操作失败：数据目录或团队共享目录的读写权限不足，请检查文件夹权限后重试。${detailsText ? `（${detailsText}）` : ''}`
	}

	if (data?.code) {
		const path = `error.${data.code}`
		let message = i18n.global.t(path, data.i18n_params ?? {})

		if (data?.code && data?.message && (data.code === 4016 || data.code === 4017 || data.code === 4018 || data.code === 4019 || data.code === 4024)) {
			message += '\n' + data.message
		}

		// If message and path are equal no translation exists for that error code
		if (path !== message) {
			return message
		}
	}
	
	// v2 errors are RFC 9457 problem+json, which carries `detail` instead of `message`.
	let message = data?.message || data?.detail || r.message || ''
	const details = Array.isArray(data?.errors)
		? data.errors
			.map(item => item?.message || item?.detail)
			.filter(detail => typeof detail === 'string' && detail && !message.includes(detail))
		: []
	if (details.length) {
		message += `${message ? '：' : ''}${details.join('；')}`
	}
	
	const causeMessage = r.cause?.response?.data?.message ?? r.cause?.message
	if (typeof causeMessage !== 'undefined') {
		message += ' ' + causeMessage
	}

	return message
}

export function translatedError(key: string): Error {
	return new Error(i18n.global.t(key))
}

export interface Action {
	title: string,
	callback: () => void,
}

export function error(e, actions: Action[] = []) {
	notify({
		type: 'error',
		title: i18n.global.t('error.error'),
		text: getErrorText(e),
		ignoreDuplicates: true,
		data: {
			actions: actions,
		},
	})
}

export function success(e, actions: Action[] = []) {
	notify({
		type: 'success',
		title: i18n.global.t('error.success'),
		text: getErrorText(e),
		ignoreDuplicates: true,
		data: {
			actions: actions,
		},
	})
}
