import {TASK_STATUS_OPTIONS, type TaskStatus} from '@/types/ITaskStatus'

export const allTaskStatuses = TASK_STATUS_OPTIONS.map(option => option.value)
export function legacyTaskStatusSelection(scope: string): TaskStatus[] {
	return scope === 'all' ? [...allTaskStatuses] : scope === 'done' ? ['done'] : ['to-do', 'doing', 'hold']
}
export function taskMatchesStatuses(task: {done?: boolean, status?: string}, selected: readonly TaskStatus[]) {
	const state = task.done ? 'done' : task.status === 'doing' || task.status === 'hold' ? task.status : 'to-do'
	return selected.includes(state)
}
export function taskStatusFilterExpression(selected: readonly TaskStatus[]) {
	if (!selected.length) return 'id = 0'
	if (allTaskStatuses.every(state => selected.includes(state))) return ''
	if (!selected.includes('done') && ['to-do', 'doing', 'hold'].every(state => selected.includes(state as TaskStatus))) return 'done = false'
	const clauses = selected.map(state => state === 'done' ? 'done = true'
		: state === 'to-do' ? 'done = false && status != \'doing\' && status != \'hold\''
			: `done = false && status = '${state}'`)
	const expression = clauses.map(clause => `(${clause})`).join(' || ')
	return '(' + expression + ')'
}
