import type {Task, TaskTraceTeamBindingStatus} from '@/client/generated'
export type ProgressTask = Task & {id: number}
export type ProgressRow = {task: ProgressTask, depth: number}

function addProgressPerson(result: Map<string, string>, username?: string) {
	const value = username?.trim()
	if (!value) return
	const key = value.toLocaleLowerCase()
	if (!result.has(key)) result.set(key, value)
}

// Team bindings keep the original owner and remote assignees. The local task's
// created_by field may instead be the account that imported the shared task.
export function progressTaskPeople(
	task: ProgressTask,
	binding?: Pick<TaskTraceTeamBindingStatus, 'owner' | 'members' | 'permission_targets'>,
	currentUsername = '',
) {
	const people = new Map<string, string>()
	if (binding) {
		addProgressPerson(people, binding.owner)
		const target = binding.permission_targets?.find(item => item.kind === 'task' && item.task_id === task.id)
		for (const permission of target?.permissions ?? []) {
			if (permission.read || permission.write || permission.owner || permission.assignee) addProgressPerson(people, permission.username)
		}
		if (!target?.permissions?.length) for (const member of binding.members ?? []) addProgressPerson(people, member)
	} else {
		addProgressPerson(people, task.created_by?.username)
		for (const assignee of task.assignees ?? []) addProgressPerson(people, assignee.username)
	}
	if (people.size === 0) addProgressPerson(people, currentUsername)
	return [...people.values()]
}
export function groupProgressTasks(tasks: ProgressTask[]) {
	const byId = new Map(tasks.map(task => [task.id, task]))
	const children = new Map<number, ProgressTask[]>()
	const roots: ProgressTask[] = []
	for (const task of tasks) {
		const parent = (task.related_tasks?.parenttask || []).map(parent => parent.id).filter((id): id is number => !!id && id !== task.id && byId.has(id)).sort((a, b) => a - b)[0]
		if (parent) children.set(parent, [...(children.get(parent) || []), task])
		else roots.push(task)
	}
	const visited = new Set<number>()
	const groups: {root: ProgressTask, rows: ProgressRow[]}[] = []
	function append(root: ProgressTask) {
		if (visited.has(root.id)) return
		const rows: ProgressRow[] = []
		function visit(task: ProgressTask, depth: number) {
			if (visited.has(task.id)) return
			visited.add(task.id); rows.push({task, depth})
			for (const child of children.get(task.id) || []) visit(child, depth + 1)
		}
		visit(root, 0); groups.push({root, rows})
	}
	roots.forEach(append)
	// A malformed/cross-linked graph must not silently hide any task.
	tasks.forEach(append)
	return groups
}
let active = 0
const waiting: (() => void)[] = []
export async function queueProgressRead<T>(read: () => Promise<T>): Promise<T> {
	if (active >= 6) await new Promise<void>(resolve => waiting.push(resolve))
	else active++
	try { return await read() } finally {
		const next = waiting.shift()
		if (next) next(); else active--
	}
}

// Keep the ancestor chain when filtering, then hide only collapsed descendants.
export function visibleProgressRows(rows: ProgressRow[], scope: string, search: string, collapsed: Set<number>, eligibleTaskIds: ReadonlySet<number> | null = null) {
	const included = new Set<number>()
	const ancestors: number[] = []
	const query = search.trim().toLocaleLowerCase()
	for (const row of rows) {
		ancestors.length = row.depth
		ancestors[row.depth] = row.task.id
		if ((eligibleTaskIds === null || eligibleTaskIds.has(row.task.id)) &&
			(!query || row.task.title?.toLocaleLowerCase().includes(query)) &&
			(scope === 'all' || (scope === 'done' ? row.task.done : !row.task.done))) {
			ancestors.forEach(id => included.add(id))
		}
	}
	let hiddenBelow = Infinity
	return rows.filter(row => {
		if (row.depth > hiddenBelow) return false
		hiddenBelow = Infinity
		if (!included.has(row.task.id)) return false
		if (!query && collapsed.has(row.task.id)) hiddenBelow = row.depth
		return true
	})
}

const DAY_MS = 86400000

function calendarDay(date: Date) {
	return Date.UTC(date.getFullYear(), date.getMonth(), date.getDate()) / DAY_MS
}

function progressDay(value: string) {
	const match = value.match(/^(\d{4})-(\d{2})-(\d{2})$/)
	if (!match) return null
	const day = Date.UTC(Number(match[1]), Number(match[2]) - 1, Number(match[3]))
	const parsed = new Date(day)
	if (parsed.getUTCFullYear() !== Number(match[1]) || parsed.getUTCMonth() !== Number(match[2]) - 1 || parsed.getUTCDate() !== Number(match[3])) return null
	return day / DAY_MS
}

export function latestProgressDate(notes: ReadonlyArray<{daily: boolean, date: string}>, today = new Date()) {
	if (Number.isNaN(+today)) return ''
	const end = calendarDay(today)
	let latestDay = -Infinity
	let latestDate = ''
	for (const note of notes) {
		if (!note.daily) continue
		const day = progressDay(note.date)
		if (day !== null && day <= end && day > latestDay) {
			latestDay = day
			latestDate = note.date
		}
	}
	return latestDate
}

export function recentProgressTaskIds(latestProgressDates: Readonly<Record<number, string>>, days: number, today = new Date()) {
	const result = new Set<number>()
	if (!Number.isSafeInteger(days) || days <= 0 || Number.isNaN(+today)) return result
	const end = calendarDay(today)
	const start = end - days + 1
	for (const [taskId, value] of Object.entries(latestProgressDates)) {
		const day = progressDay(value)
		if (day !== null && day >= start && day <= end) result.add(Number(taskId))
	}
	return result
}
