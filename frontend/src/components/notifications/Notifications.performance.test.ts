import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest'
import {flushPromises, mount, type VueWrapper} from '@vue/test-utils'
import {nextTick, type Ref} from 'vue'
import Notifications from './Notifications.vue'
import NotificationModel from '@/models/notification'
import UserModel from '@/models/user'
import TaskModel from '@/models/task'
import {error as showError} from '@/message'

const mocks = vi.hoisted(() => ({
	read: vi.fn(), totalPages: 1, deleteOne: vi.fn(), team: undefined as any, markAllRead: vi.fn(), remove: vi.fn(), update: vi.fn(), routerPush: vi.fn(),
	connected: undefined as unknown as Ref<boolean>,
	subscriber: undefined as unknown as (message: {event: string, data: unknown}) => void,
}))
vi.mock('@/services/notification', () => ({default: class {
	getAll = mocks.read
	get totalPages() { return mocks.totalPages }
	markAllRead = mocks.markAllRead
	delete = mocks.remove
	update = mocks.update
}}))
vi.mock('@/composables/useWebSocket', async () => {
	const {ref} = await import('vue')
	mocks.connected = ref(true)
	return {useWebSocket: () => ({connected: mocks.connected, subscribe: (_name: string, subscriber: typeof mocks.subscriber) => {mocks.subscriber = subscriber; return () => {}}})}
})
vi.mock('@/stores/auth', () => ({useAuthStore: () => ({info: {id: 1, username: 'me'}, settings: {frontendSettings: {}}})}))
vi.mock('@/helpers/tasktraceLocal', () => ({isLocalBuild: true}))
vi.mock('@/client/generated', () => ({notificationsDelete: mocks.deleteOne}))
vi.mock('@/stores/tasktraceUpdate', () => ({useTasktraceUpdateStore: () => ({state: {}, supported: false, shouldNotify: false})}))
vi.mock('@/stores/tasktraceTeam', async () => {
	const {reactive} = await import('vue')
	mocks.team = reactive({
		conflictCount: 0, status: {notifications: [] as {id: string, read?: boolean}[]}, avatarFor: () => '', dismissNotifications: vi.fn(), markNotificationsRead: vi.fn(), refresh: vi.fn(), sync: vi.fn(),
		get notificationCount() { return this.status.notifications.length },
		get unreadNotificationCount() { return this.status.notifications.filter(notice => !notice.read).length },
		get activityCount() { return this.unreadNotificationCount },
	})
	return {useTasktraceTeamStore: () => mocks.team}
})
vi.mock('vue-router', () => ({useRouter: () => ({push: mocks.routerPush}), isNavigationFailure: (failure: {type?: number}, type: number) => failure?.type === type, NavigationFailureType: {duplicated: 16}}))
vi.mock('vue-i18n', async original => ({...(await original<typeof import('vue-i18n')>()), useI18n: () => ({t: (key: string) => key})}))
vi.mock('@/components/tasks/partials/commentReplyContext', () => ({scrollAndHighlightComment: vi.fn()}))
vi.mock('@/message', () => ({success: vi.fn(), error: vi.fn()}))
vi.mock('@/helpers/time/formatDate', async original => ({...(await original<typeof import('@/helpers/time/formatDate')>()), formatDateLong: () => 'Date', formatDisplayDate: () => 'Date'}))

let wrapper: VueWrapper | undefined
let visible = true
const notice = (id: number) => new NotificationModel({id, name: 'test', notification: {doer: new UserModel({id: 1, username: 'tester'}), task: new TaskModel({id, title: 'Task'})}, created: new Date('2026-09-22T00:00:00Z')})
function start() {
	wrapper = mount(Notifications, {global: {
		mocks: {$t: (key: string) => key},
		directives: {tooltip: {}},
		stubs: {Icon: true, BaseButton: {template: '<button><slot /></button>'}, CustomTransition: {template: '<div><slot /></div>'}, User: true, Modal: true, XButton: true, VersionChanges: true, TeamMemberIdentity: true},
	}})
}
async function show() {await wrapper!.get('.trigger-button').trigger('click'); await flushPromises()}
beforeEach(() => {
	vi.clearAllMocks(); mocks.routerPush.mockReset().mockResolvedValue(undefined); mocks.team.markNotificationsRead.mockReset(); mocks.team.refresh.mockReset(); mocks.team.sync.mockReset(); mocks.team.conflictCount = 0; mocks.totalPages = 1; mocks.team.status.notifications = []; visible = true; mocks.connected.value = true
	vi.spyOn(document, 'visibilityState', 'get').mockImplementation(() => visible ? 'visible' : 'hidden')
	vi.spyOn(document, 'hidden', 'get').mockImplementation(() => !visible)
})
afterEach(() => {wrapper?.unmount(); wrapper = undefined; vi.restoreAllMocks()})

describe('notification refresh ordering', () => {
	it('retains initial history when a websocket notification arrives during the first read', async () => {
		let finish!: (value: NotificationModel[]) => void
		mocks.read.mockImplementationOnce(() => new Promise(resolve => {finish = resolve})).mockResolvedValueOnce([notice(2), notice(1)])
		start(); await flushPromises()
		mocks.subscriber({event: 'notification.created', data: notice(2)})
		finish([notice(1)])
		await flushPromises()
		await show()
		expect(mocks.read).toHaveBeenCalledTimes(2)
		expect(wrapper!.findAll('.single-notification')).toHaveLength(2)
	})
	it('catches up after a hidden websocket disconnect and reconnect without waiting for an interval', async () => {
		mocks.read.mockResolvedValueOnce([notice(1)]).mockResolvedValueOnce([notice(2), notice(1)])
		start(); await flushPromises()
		visible = false; document.dispatchEvent(new Event('visibilitychange'))
		mocks.connected.value = false; await nextTick()
		mocks.connected.value = true; await nextTick()
		expect(mocks.read).toHaveBeenCalledTimes(1)
		visible = true; document.dispatchEvent(new Event('visibilitychange')); window.dispatchEvent(new Event('focus'))
		await flushPromises(); await show()
		expect(mocks.read).toHaveBeenCalledTimes(2)
		expect(wrapper!.findAll('.single-notification')).toHaveLength(2)
	})
	it('does not start a follow-up request after an in-flight component is unmounted', async () => {
		let finish!: (value: NotificationModel[]) => void
		mocks.read.mockImplementationOnce(() => new Promise(resolve => {finish = resolve}))
		start(); await flushPromises(); mocks.subscriber({event: 'notification.created', data: notice(2)})
		wrapper!.unmount(); wrapper = undefined
		finish([notice(1)]); await flushPromises()
		expect(mocks.read).toHaveBeenCalledTimes(1)
	})
})

describe('reading and deleting notifications', () => {
	it('keeps visible rows and scroll position when a websocket notification arrives', async () => {
		mocks.read.mockResolvedValue([notice(2), notice(1)])
		start(); await flushPromises(); await show()
		const popup = wrapper!.get('.notifications-list').element as HTMLElement
		popup.scrollTop = 160
		const first = wrapper!.findAll('.single-notification')[0].element
		mocks.subscriber({event: 'notification.created', data: notice(3)})
		await nextTick()
		expect(wrapper!.findAll('.single-notification')).toHaveLength(2)
		expect(wrapper!.findAll('.single-notification')[0].element).toBe(first)
		expect(popup.scrollTop).toBe(160)
		await wrapper!.get('.reading-status button').trigger('click')
		await nextTick()
		expect(wrapper!.findAll('.single-notification')).toHaveLength(3)
	})
	it('removes only the selected notification and ignores an old websocket replay', async () => {
		mocks.read.mockResolvedValue([notice(2), notice(1)])
		mocks.deleteOne.mockResolvedValue({})
		start(); await flushPromises(); await show()
		await wrapper!.findAll('.notification-delete')[0].trigger('click'); await flushPromises()
		expect(mocks.deleteOne).toHaveBeenCalledWith({path: {notificationid: 2}})
		expect(wrapper!.findAll('.single-notification')).toHaveLength(1)
		mocks.subscriber({event: 'notification.created', data: notice(2)}); await nextTick()
		await wrapper!.get('.trigger-button').trigger('click'); await show()
		expect(wrapper!.findAll('.single-notification')).toHaveLength(1)
	})
	it('keeps the notification when deletion fails and allows retry', async () => {
		mocks.read.mockResolvedValue([notice(1)])
		mocks.deleteOne.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce({})
		start(); await flushPromises(); await show()
		await wrapper!.get('.notification-delete').trigger('click'); await flushPromises()
		expect(wrapper!.findAll('.single-notification')).toHaveLength(1)
		expect(wrapper!.get('.notification-delete').attributes('disabled')).toBeUndefined()
		await wrapper!.get('.notification-delete').trigger('click'); await flushPromises()
		expect(wrapper!.findAll('.single-notification')).toHaveLength(0)
	})
	it('appends older history without discarding the rows being read', async () => {
		mocks.totalPages = 2
		mocks.read.mockImplementation((_model, _params, page) => Promise.resolve(page === 2 ? [notice(1)] : [notice(2)]))
		start(); await flushPromises(); await show()
		// XButton is a stub: invoke the action through the stub's emitted click.
		const more = wrapper!.findAllComponents({name: 'XButton'}).find(button => button.attributes('class')?.includes('is-fullwidth'))
		expect(more).toBeDefined()
		more!.vm.$emit('click'); await flushPromises()
		expect(mocks.read).toHaveBeenLastCalledWith(undefined, {per_page: 50}, 2)
		expect(wrapper!.findAll('.single-notification')).toHaveLength(2)
	})
	it('freezes incoming team notifications and deletes one by id', async () => {
		mocks.read.mockResolvedValue([])
		mocks.team.status.notifications = [{id: 'first', actor: 'Alice', task_title: '已显示事项'}]
		mocks.team.dismissNotifications.mockImplementation(async (ids: string[]) => {
			mocks.team.status.notifications = mocks.team.status.notifications.filter((item: {id: string}) => !ids.includes(item.id))
		})
		start(); await flushPromises(); await show()
		mocks.team.status.notifications = [{id: 'new', actor: 'Bob', task_title: '新事项'}, ...mocks.team.status.notifications]
		await nextTick()
		expect(wrapper!.text()).not.toContain('新事项')
		await wrapper!.get('[aria-label="删除此条团队通知"]').trigger('click'); await flushPromises()
		expect(mocks.team.dismissNotifications).toHaveBeenCalledWith(['first'])
		expect(wrapper!.findAll('.team-notification-row')).toHaveLength(0)
		await wrapper!.get('.reading-status button').trigger('click'); await nextTick()
		expect(wrapper!.text()).toContain('新事项')
	})
})


describe('team notification read receipts', () => {
	function prepare() {
		mocks.read.mockResolvedValue([])
		mocks.team.status.notifications = [{id: 'first', actor: 'Alice', task_title: 'Keep history', task_id: 12, comment_id: 34}, {id: 'second', actor: 'Bob', task_title: 'Other update', task_id: 13}]
		mocks.team.markNotificationsRead.mockImplementation(async (ids: string[] = []) => {
			mocks.team.status.notifications = mocks.team.status.notifications.map((n: {id: string}) => ids.length === 0 || ids.includes(n.id) ? {...n, read: true} : n)
		})
		vi.spyOn(document, 'getElementById').mockReturnValue(document.createElement('div'))
	}
	it('opens the comment and removes only its unread dot while retaining history', async () => {
		prepare(); start(); await flushPromises(); await show()
		expect(wrapper!.findAll('.team-notification-row .read-indicator:not(.read)')).toHaveLength(2)
		await wrapper!.findAll('.team-task-link')[0].trigger('click'); await flushPromises()
		expect(mocks.routerPush).toHaveBeenCalledWith({name: 'task.detail', params: {id: 12}, hash: '#comment-34'})
		expect(mocks.team.markNotificationsRead).toHaveBeenCalledWith(['first'])
		expect(mocks.team.dismissNotifications).not.toHaveBeenCalled()
		await show()
		expect(wrapper!.findAll('.team-notification-row')).toHaveLength(2)
		expect(wrapper!.findAll('.team-notification-row .read-indicator.read')).toHaveLength(1)
		expect(wrapper!.findAll('.team-notification-row .read-indicator:not(.read)')).toHaveLength(1)
		await wrapper!.findAll('.team-task-link')[0].trigger('click'); await flushPromises()
		expect(mocks.team.markNotificationsRead).toHaveBeenCalledTimes(1)
	})
	it('does not mark as read when navigation is cancelled', async () => {
		prepare(); mocks.routerPush.mockResolvedValue({type: 4})
		start(); await flushPromises(); await show()
		await wrapper!.findAll('.team-task-link')[0].trigger('click'); await flushPromises()
		expect(mocks.team.markNotificationsRead).not.toHaveBeenCalled()
		expect(mocks.team.dismissNotifications).not.toHaveBeenCalled()
	})
	it('marks an already open comment as read without deleting it', async () => {
		prepare(); mocks.routerPush.mockResolvedValue({type: 16})
		start(); await flushPromises(); await show()
		await wrapper!.findAll('.team-task-link')[0].trigger('click'); await flushPromises()
		expect(mocks.team.markNotificationsRead).toHaveBeenCalledWith(['first'])
		expect(mocks.team.status.notifications).toHaveLength(2)
	})
	it('mark all read preserves rows and clears the unread badge', async () => {
		prepare(); start(); await flushPromises(); await show()
		const button = wrapper!.findAllComponents({name: 'XButton'}).find(item => item.attributes('class')?.includes('mbs-2'))
		expect(button).toBeDefined(); button!.vm.$emit('click'); await flushPromises()
		expect(mocks.team.markNotificationsRead).toHaveBeenCalledWith()
		expect(mocks.team.dismissNotifications).not.toHaveBeenCalled()
		expect(wrapper!.findAll('.team-notification-row')).toHaveLength(2)
		expect(wrapper!.findAll('.team-notification-row .read-indicator.read')).toHaveLength(2)
		expect(wrapper!.find('.unread-indicator').exists()).toBe(false)
	})
	it('does not hide the unread dot or delete history if persisting read state fails', async () => {
		prepare(); mocks.team.markNotificationsRead.mockRejectedValueOnce(new Error('disk unavailable'))
		start(); await flushPromises(); await show()
		await wrapper!.findAll('.team-task-link')[0].trigger('click'); await flushPromises(); await show()
		expect(wrapper!.findAll('.team-notification-row .read-indicator:not(.read)')).toHaveLength(2)
		expect(mocks.team.dismissNotifications).not.toHaveBeenCalled()
	})
})

// Target metadata can arrive after a notification, or remain unavailable when a
// LAN peer is offline. These states are independent of collaboration conflicts.
describe('member update navigation', () => {
	function prepare() {
		mocks.read.mockResolvedValue([])
		mocks.team.conflictCount = 1
		mocks.team.status.notifications = [{id: 'unresolved', actor: 'Alice', task_title: '待同步的更新', node_id: 'shared-node', shared_comment_id: 'shared-comment'}]
		vi.spyOn(document, 'getElementById').mockReturnValue(document.createElement('div'))
		return vi.spyOn(window, 'dispatchEvent')
	}
	const resolvesTarget = () => {
		mocks.team.status.notifications = [{...mocks.team.status.notifications[0], task_id: 12, comment_id: 34}]
	}
	it('keeps an unresolved update unread and never opens conflict handling', async () => {
		const dispatch = prepare()
		start(); await flushPromises(); await show()
		await wrapper!.get('.team-task-link').trigger('click'); await flushPromises()
		expect(mocks.team.refresh).toHaveBeenCalledWith(true)
		expect(mocks.team.sync).toHaveBeenCalledTimes(1)
		expect(mocks.routerPush).not.toHaveBeenCalled()
		expect(dispatch).not.toHaveBeenCalled()
		expect(showError).toHaveBeenCalledWith(expect.objectContaining({message: expect.stringContaining('暂时无法定位')}))
		expect(wrapper!.find('.notifications-list').exists()).toBe(true)
		expect(wrapper!.find('.team-task-link:disabled').exists()).toBe(false)
		expect(mocks.team.markNotificationsRead).not.toHaveBeenCalled()
		expect(mocks.team.dismissNotifications).not.toHaveBeenCalled()
	})
	it.each(['current status', 'refresh', 'sync'])('opens the resolved comment from %s despite a frozen reading list and existing conflicts', async source => {
		const dispatch = prepare()
		start(); await flushPromises(); await show()
		if (source === 'current status') resolvesTarget()
		if (source === 'refresh') mocks.team.refresh.mockImplementationOnce(resolvesTarget)
		if (source === 'sync') mocks.team.sync.mockImplementationOnce(resolvesTarget)
		await wrapper!.get('.team-task-link').trigger('click'); await flushPromises()
		expect(mocks.routerPush).toHaveBeenCalledExactlyOnceWith({name: 'task.detail', params: {id: 12}, hash: '#comment-34'})
		expect(mocks.team.markNotificationsRead).toHaveBeenCalledWith(['unresolved'])
		expect(mocks.team.sync).toHaveBeenCalledTimes(source === 'sync' ? 1 : 0)
		expect(mocks.team.refresh).toHaveBeenCalledTimes(source === 'current status' ? 0 : 1)
		expect(dispatch).not.toHaveBeenCalled()
		expect(mocks.team.dismissNotifications).not.toHaveBeenCalled()
	})
	it('reports a synchronization failure without navigation or marking read', async () => {
		const dispatch = prepare()
		mocks.team.sync.mockRejectedValueOnce(new Error('Share is offline'))
		start(); await flushPromises(); await show()
		await wrapper!.get('.team-task-link').trigger('click'); await flushPromises()
		expect(dispatch).not.toHaveBeenCalled()
		expect(mocks.routerPush).not.toHaveBeenCalled()
		expect(showError).toHaveBeenCalledWith(expect.objectContaining({message: expect.stringContaining('网络连接')}))
		expect(mocks.team.markNotificationsRead).not.toHaveBeenCalled()
		expect(wrapper!.find('.notifications-list').exists()).toBe(true)
	})
	it('does not mark an update read when its comment is still missing', async () => {
		const dispatch = prepare()
		mocks.team.status.notifications[0].task_id = 12
		start(); await flushPromises(); await show()
		await wrapper!.get('.team-task-link').trigger('click'); await flushPromises()
		expect(mocks.routerPush).not.toHaveBeenCalled()
		expect(dispatch).not.toHaveBeenCalled()
		expect(mocks.team.markNotificationsRead).not.toHaveBeenCalled()
	})
	it('opens conflict handling only from the explicit conflict entry', async () => {
		const dispatch = prepare()
		start(); await flushPromises(); await show()
		await wrapper!.get('button.team-notification-row').trigger('click'); await flushPromises()
		expect(dispatch).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({type: 'tasktrace-team-conflicts-open'}))
		expect(mocks.routerPush).not.toHaveBeenCalled()
		expect(mocks.team.refresh).not.toHaveBeenCalled()
		expect(mocks.team.sync).not.toHaveBeenCalled()
	})
})
