import {calendarDate} from './reminderRepeat'
export function reviewDefaults(now = new Date()) {
	const from = new Date(now); from.setDate(from.getDate() - 6)
	return {mode: 'days' as 'days' | 'dates', days: 7, from: calendarDate(from), to: calendarDate(now)}
}
export function localReviewTimestamp(date: Date) {
	const offset = -date.getTimezoneOffset()
	const sign = offset < 0 ? '-' : '+'
	return calendarDate(date) + 'T00:00:00' + sign + String(Math.floor(Math.abs(offset) / 60)).padStart(2, '0') + ':' + String(Math.abs(offset) % 60).padStart(2, '0')
}
export function reviewRange(value: ReturnType<typeof reviewDefaults>, now = new Date()) {
	let from = new Date(value.from + 'T00:00:00'), to = new Date(value.to + 'T00:00:00')
	if (value.mode === 'days') {
		if (!Number.isInteger(value.days) || value.days < 1 || value.days > 36500) throw new Error('过去天数请输入 1 至 36500 的整数。')
		to = new Date(now.getFullYear(), now.getMonth(), now.getDate())
		from = new Date(to); from.setDate(from.getDate() - value.days + 1)
	} else if (calendarDate(from) !== value.from || calendarDate(to) !== value.to || !Number.isFinite(+from) || !Number.isFinite(+to)) {
		throw new Error('请选择有效的开始和结束日期。')
	}
	if (from > to) throw new Error('结束日期不能早于开始日期。')
	const until = new Date(to); until.setDate(until.getDate() + 1)
	return {from: localReviewTimestamp(from), until: localReviewTimestamp(until)}
}
