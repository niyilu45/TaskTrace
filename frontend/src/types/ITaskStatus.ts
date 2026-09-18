export const TASK_STATUSES = {
	TODO: 'to-do',
	DOING: 'doing',
	DONE: 'done',
	HOLD: 'hold',
} as const

export type TaskStatus = typeof TASK_STATUSES[keyof typeof TASK_STATUSES]

export const TASK_STATUS_OPTIONS: ReadonlyArray<{value: TaskStatus, label: string}> = [
	{value: TASK_STATUSES.TODO, label: '待办'},
	{value: TASK_STATUSES.DOING, label: '进行中'},
	{value: TASK_STATUSES.DONE, label: '已完成'},
	{value: TASK_STATUSES.HOLD, label: '暂停'},
]

export function isTaskStatus(value: unknown): value is TaskStatus {
	return TASK_STATUS_OPTIONS.some(option => option.value === value)
}

export function taskStatusLabel(value: unknown, done = false): string {
	if (!isTaskStatus(value)) return done ? '已完成' : '待办'
	return TASK_STATUS_OPTIONS.find(option => option.value === value)?.label ?? '待办'
}
