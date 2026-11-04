
export interface ReminderRepeatRule {
	weekdays: number[]
	startDate: string
	endDate: string
	time: string
	times?: Record<number, string>
}

const validTime = /^([01]\d|2[0-3]):[0-5]\d$/

export const REMINDER_WEEKDAYS = [
	{value: 1, label: '周一'}, {value: 2, label: '周二'}, {value: 3, label: '周三'},
	{value: 4, label: '周四'}, {value: 5, label: '周五'}, {value: 6, label: '周六'}, {value: 0, label: '周日'},
]
export function calendarDate(date: Date): string {
	return [date.getFullYear(), String(date.getMonth() + 1).padStart(2, '0'), String(date.getDate()).padStart(2, '0')].join('-')
}
export function defaultRepeatRule(date = new Date()): ReminderRepeatRule {
	const end = new Date(date); end.setMonth(end.getMonth() + 1)
	return {weekdays: [date.getDay()], startDate: calendarDate(date), endDate: calendarDate(end), time: '09:00'}
}
export function parseRepeatRule(raw?: string): ReminderRepeatRule | null {
	try {
		const rule = JSON.parse(raw || '') as ReminderRepeatRule
		return repeatRuleError(rule) ? null : rule
	} catch { return null }
}
export function reminderTimeOnDay(rule: ReminderRepeatRule, day: number): string {
	return rule.times?.[day] ?? rule.time
}

export function firstRepeatDate(rule: ReminderRepeatRule): Date | null {
	const date = new Date(rule.startDate + 'T12:00')
	for (let i = 0; i < 7 && calendarDate(date) <= rule.endDate; i++, date.setDate(date.getDate() + 1)) {
		const day = date.getDay()
		if (rule.weekdays.includes(day)) return new Date(calendarDate(date) + 'T' + reminderTimeOnDay(rule, day))
	}
	return null
}
export function repeatRuleError(rule: ReminderRepeatRule): string {
	if (!rule || !Array.isArray(rule.weekdays) || !rule.weekdays.length || rule.weekdays.some(day => !Number.isInteger(day) || day < 0 || day > 6)) return '请至少选择一个星期。'
	for (const key of [rule.startDate, rule.endDate]) {
		if (!/^\d{4}-\d{2}-\d{2}$/.test(key) || calendarDate(new Date(key + 'T12:00')) !== key) return '请选择有效的开始和结束日期。'
	}
	if (rule.endDate < rule.startDate) return '结束日期不能早于开始日期。'
	if (!validTime.test(rule.time)) return '请选择默认提醒时间。'
	if (rule.times !== undefined && (!rule.times || typeof rule.times !== 'object' || Array.isArray(rule.times)
		|| Object.entries(rule.times).some(([day, time]) => !/^[0-6]$/.test(day) || !rule.weekdays.includes(Number(day)) || typeof time !== 'string' || !validTime.test(time)))) {
		return '请选择有效的各日提醒时间。'
	}
	if (!firstRepeatDate(rule)) return '日期范围内没有所选星期，请调整日期或星期。'
	return ''
}
export function repeatRuleText(raw?: string): string {
	const rule = parseRepeatRule(raw)
	if (!rule) return ''
	const days = REMINDER_WEEKDAYS.filter(day => rule.weekdays.includes(day.value))
	const time = reminderTimeOnDay(rule, days[0].value)
	const schedule = days.every(day => reminderTimeOnDay(rule, day.value) === time)
		? days.map(day => day.label).join('、') + ' ' + time
		: days.map(day => day.label + ' ' + reminderTimeOnDay(rule, day.value)).join('、')
	return '每周 ' + schedule + '（' + rule.startDate + ' 至 ' + rule.endDate + '）'
}
export function serializeRepeatRule(rule: ReminderRepeatRule): string {
	const weekdays = [...new Set(rule.weekdays)].sort((a, b) => a - b)
	const times = Object.fromEntries(Object.entries(rule.times || {}).filter(([, time]) => time !== rule.time))
	return JSON.stringify({weekdays, startDate: rule.startDate, endDate: rule.endDate, time: rule.time, times: Object.keys(times).length ? times : undefined})
}
