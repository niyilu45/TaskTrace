import type {Task} from '@/client/generated'
export type ProgressTask = Task & {id: number}
export type ProgressRow = {task: ProgressTask, depth: number}
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
