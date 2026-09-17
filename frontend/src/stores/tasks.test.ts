import {setActivePinia, createPinia} from 'pinia'
import {beforeEach, describe, expect, it, vi} from 'vitest'

vi.mock('@/router', () => ({
	default: {
		currentRoute: {value: {params: {}}},
		isReady: () => Promise.resolve(),
	},
}))

vi.mock('vue-i18n', () => ({
	useI18n: () => ({t: (key: string) => key}),
	createI18n: () => ({global: {t: (key: string) => key}}),
}))

vi.mock('@/stores/base', () => ({
	useBaseStore: () => ({setHasTasks: vi.fn()}),
}))

const labelQueries = vi.hoisted(() => ({
	ensureLabels: vi.fn(),
	refreshLabels: vi.fn(),
	createLabel: vi.fn(),
	createLabelMutationOptions: () => ({mutationFn: labelQueries.createLabel}),
	getLabelByExactTitle: vi.fn((labels: Array<{title?: string}>, title: string) =>
		labels.find(label => label.title?.toLowerCase() === title.toLowerCase()),
	),
}))

const labelSdk = vi.hoisted(() => ({
	taskLabelsCreate: vi.fn(),
	taskLabelsDelete: vi.fn(),
	tasksCreate: vi.fn(),
}))

vi.mock('@/client/queries/labels', () => labelQueries)
vi.mock('@/client/generated', () => labelSdk)

import {buildDefaultRemindersForQuickAdd, createTasksWithUndo, useTaskStore} from './tasks'
import TaskService from '@/services/task'
import TaskModel from '@/models/task'
import UserModel from '@/models/user'
import TaskReminderModel from '@/models/taskReminder'
import {useKanbanStore} from './kanban'
import {REMINDER_PERIOD_RELATIVE_TO_TYPES} from '@/types/IReminderPeriodRelativeTo'
import type {ITaskReminder} from '@/modelTypes/ITaskReminder'
import type {IBucket} from '@/modelTypes/IBucket'
import type {ITask} from '@/modelTypes/ITask'

const aDefault: ITaskReminder = {
	reminder: null,
	relativePeriod: -3600,
	relativeTo: REMINDER_PERIOD_RELATIVE_TO_TYPES.DUEDATE,
} as ITaskReminder

describe('buildDefaultRemindersForQuickAdd', () => {
	it('returns empty array when due date is null', () => {
		expect(buildDefaultRemindersForQuickAdd([aDefault], null)).toEqual([])
	})

	it('returns empty array when defaults are undefined', () => {
		expect(buildDefaultRemindersForQuickAdd(undefined, '2026-05-01T00:00:00.000Z')).toEqual([])
	})

	it('returns empty array when defaults are empty', () => {
		expect(buildDefaultRemindersForQuickAdd([], '2026-05-01T00:00:00.000Z')).toEqual([])
	})

	it('clones defaults with relativeTo locked to due_date', () => {
		const result = buildDefaultRemindersForQuickAdd([aDefault], '2026-05-01T00:00:00.000Z')
		expect(result).toHaveLength(1)
		expect(result[0].relativePeriod).toBe(-3600)
		expect(result[0].relativeTo).toBe(REMINDER_PERIOD_RELATIVE_TO_TYPES.DUEDATE)
		expect(result[0].reminder).toBeNull()
	})

	it('does not share references with the input array', () => {
		const defaults = [aDefault]
		const result = buildDefaultRemindersForQuickAdd(defaults, '2026-05-01T00:00:00.000Z')
		expect(result[0]).not.toBe(defaults[0])
	})

	it('forces relativeTo to due_date even if a default somehow had another value', () => {
		const weird = {...aDefault, relativeTo: REMINDER_PERIOD_RELATIVE_TO_TYPES.STARTDATE} as ITaskReminder
		const result = buildDefaultRemindersForQuickAdd([weird], '2026-05-01T00:00:00.000Z')
		expect(result[0].relativeTo).toBe(REMINDER_PERIOD_RELATIVE_TO_TYPES.DUEDATE)
	})
})

describe('ensureLabelsExist', () => {
	beforeEach(() => {
		setActivePinia(createPinia())
		labelQueries.ensureLabels.mockReset()
		labelQueries.refreshLabels.mockReset().mockImplementation(() => labelQueries.ensureLabels())
		labelQueries.createLabel.mockReset()
	})

	it('skips labels that fail to create and returns the resolved ones', async () => {
		const taskStore = useTaskStore()
		labelQueries.ensureLabels.mockResolvedValue([{id: 1, title: 'existing'}])
		labelQueries.createLabel.mockImplementation(async label => {
			if (label.title === 'forbidden') {
				throw new Error('403')
			}
			return {id: 99, title: label.title}
		})

		const result = await taskStore.ensureLabelsExist(['existing', 'created', 'forbidden'])
		const titles = result.map(l => l.title)

		expect(titles).toContain('existing')
		expect(titles).toContain('created')
		expect(titles).not.toContain('forbidden')
		expect(result).toHaveLength(2)
	})

	it('loads the labels before creating unknown ones and reuses what it finds', async () => {
		const taskStore = useTaskStore()
		labelQueries.ensureLabels.mockResolvedValue([{id: 1, title: 'foo'}, {id: 2, title: 'bar'}])

		const result = await taskStore.ensureLabelsExist(['foo', 'bar'])

		expect(labelQueries.createLabel).not.toHaveBeenCalled()
		expect(result.map(l => l.id).sort()).toEqual([1, 2])
	})

	it('deduplicates label titles before resolving them', async () => {
		const taskStore = useTaskStore()
		labelQueries.ensureLabels.mockResolvedValue([{id: 1, title: 'foo'}])

		const result = await taskStore.ensureLabelsExist(['foo', 'foo'])

		expect(result).toEqual([{id: 1, title: 'foo'}])
	})

	it('still creates the label when loading them fails', async () => {
		const taskStore = useTaskStore()
		labelQueries.ensureLabels.mockRejectedValue(new Error('nope'))
		labelQueries.createLabel.mockImplementation(async label => ({id: 42, title: label.title}))

		const result = await taskStore.ensureLabelsExist(['foo'])

		expect(result.map(l => l.title)).toEqual(['foo'])
	})

	it('revalidates stale labels before creating missing titles', async () => {
		const taskStore = useTaskStore()
		const remoteLabel = {id: 2, title: 'remote'}
		labelQueries.ensureLabels.mockResolvedValue([{id: 1, title: 'cached'}])
		labelQueries.refreshLabels.mockResolvedValue([{id: 1, title: 'cached'}, remoteLabel])

		const result = await taskStore.ensureLabelsExist(['remote'])

		expect(labelQueries.refreshLabels).toHaveBeenCalledOnce()
		expect(labelQueries.createLabel).not.toHaveBeenCalled()
		expect(result).toEqual([remoteLabel])
	})
})

describe('task label operations', () => {
	beforeEach(() => {
		setActivePinia(createPinia())
		labelSdk.taskLabelsCreate.mockReset()
		labelSdk.taskLabelsDelete.mockReset()
	})

	it('adds a label with the generated task-label operation', async () => {
		labelSdk.taskLabelsCreate.mockResolvedValue({data: {label_id: 4}})
		const taskStore = useTaskStore()
		const kanbanStore = useKanbanStore()
		kanbanStore.setBuckets([{
			id: 1,
			tasks: [{id: 7, labels: []} as unknown as ITask],
		} as IBucket])

		await taskStore.addLabel({taskId: 7, label: {id: 4, title: 'label'}})

		expect(labelSdk.taskLabelsCreate).toHaveBeenCalledWith({
			path: {task: 7},
			body: {label_id: 4},
		})
		expect(kanbanStore.buckets[0].tasks[0].labels).toEqual([{id: 4, title: 'label'}])
	})

	it('removes a label with the generated task-label operation', async () => {
		labelSdk.taskLabelsDelete.mockResolvedValue({data: undefined})
		const taskStore = useTaskStore()
		const kanbanStore = useKanbanStore()
		kanbanStore.setBuckets([{
			id: 1,
			tasks: [{id: 7, labels: [{id: 4, title: 'label'}]} as unknown as ITask],
		} as IBucket])

		await taskStore.removeLabel({taskId: 7, label: {id: 4, title: 'label'}})

		expect(labelSdk.taskLabelsDelete).toHaveBeenCalledWith({
			path: {task: 7, label: 4},
		})
		expect(kanbanStore.buckets[0].tasks[0].labels).toEqual([])
	})
})

describe('local task creation with undo', () => {
	beforeEach(() => { labelSdk.tasksCreate.mockReset() })
	const headers = {'X-TaskTrace-Undo': '1', 'X-TaskTrace-Undo-Group': '9de5a1c4-d4f8-408c-aad8-d12b8ebdc54e'}

	it('preserves wire conversion, input positions and one group with sequential writes', async () => {
		let active = 0
		let maximum = 0
		labelSdk.tasksCreate.mockImplementation(async ({path, body}) => {
			active++; maximum = Math.max(maximum, active)
			await Promise.resolve()
			active--
			return {data: {...body, id: body.title === 'First' ? 101 : 102, project_id: path.project}}
		})
		const first = new TaskModel({
			title: 'First', projectId: 7, priority: 3,
			dueDate: new Date('2026-09-20T12:00:00Z'),
			assignees: [new UserModel({id: 5, username: 'member'})],
			reminders: [new TaskReminderModel({reminder: null, relativePeriod: -3600, relativeTo: REMINDER_PERIOD_RELATIVE_TO_TYPES.DUEDATE})],
		})
		const result = await createTasksWithUndo(new TaskService(), [first, new TaskModel({title: 'Second', projectId: 8})], headers)
		expect(result.error).toBeNull()
		expect(result.tasks.map(task => task?.id)).toEqual([101, 102])
		expect(result.tasks[0]?.dueDate).toEqual(first.dueDate)
		expect(result.tasks[0]?.projectId).toBe(7)
		expect(maximum).toBe(1)
		expect(labelSdk.tasksCreate.mock.calls.map(([request]) => request.body.title)).toEqual(['Second', 'First'])
		const request = labelSdk.tasksCreate.mock.calls[1][0]
		expect(request.headers).toBe(headers)
		expect(request.body).toMatchObject({due_date: '2026-09-20T12:00:00.000Z', priority: 3, assignees: [{id: 5, username: 'member'}], reminders: [{reminder: null, relative_period: -3600, relative_to: 'due_date'}]})
		expect(request.body).not.toHaveProperty('max_permission')
		expect(request.body).not.toHaveProperty('reminder_dates')
		expect(labelSdk.tasksCreate.mock.calls[0][0].headers).toBe(headers)
	})

	it('stops at the first failure and retains aligned successful results for retry', async () => {
		const failure = new Error('Offline')
		labelSdk.tasksCreate.mockResolvedValueOnce({data: {id: 103, title: 'Third', project_id: 7}}).mockRejectedValueOnce(failure)
		const result = await createTasksWithUndo(new TaskService(), ['First', 'Second', 'Third'].map(title => new TaskModel({title, projectId: 7})), headers)
		expect(result.error).toBe(failure)
		expect(result.tasks.map(task => task?.id || null)).toEqual([null, null, 103])
		expect(labelSdk.tasksCreate).toHaveBeenCalledTimes(2)
	})
})