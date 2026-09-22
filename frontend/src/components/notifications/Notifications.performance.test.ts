import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest'
import {flushPromises, mount, type VueWrapper} from '@vue/test-utils'
import {nextTick, type Ref} from 'vue'
import Notifications from './Notifications.vue'
import NotificationModel from '@/models/notification'
import UserModel from '@/models/user'
import TaskModel from '@/models/task'

const mocks = vi.hoisted(() => ({
	read: vi.fn(), markAllRead: vi.fn(), remove: vi.fn(), update: vi.fn(),
	connected: undefined as unknown as Ref<boolean>,
	subscriber: undefined as unknown as (message: {event: string, data: unknown}) => void,
}))
vi.mock('@/services/notification', () => ({default: class {
	getAll = mocks.read
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
vi.mock('@/helpers/tasktraceLocal', () => ({isLocalBuild: false}))
vi.mock('@/stores/tasktraceUpdate', () => ({useTasktraceUpdateStore: () => ({state: {}, supported: false, shouldNotify: false})}))
vi.mock('@/stores/tasktraceTeam', () => ({useTasktraceTeamStore: () => ({activityCount: 0, notificationCount: 0, status: {notifications: []}})}))
vi.mock('vue-router', () => ({useRouter: () => ({push: vi.fn()}), isNavigationFailure: () => false, NavigationFailureType: {duplicated: 16}}))
vi.mock('vue-i18n', async original => ({...(await original<typeof import('vue-i18n')>()), useI18n: () => ({t: (key: string) => key})}))
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
	vi.clearAllMocks(); visible = true; mocks.connected.value = true
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