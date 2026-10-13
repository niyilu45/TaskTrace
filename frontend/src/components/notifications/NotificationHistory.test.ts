import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest'
import {flushPromises, mount, type VueWrapper} from '@vue/test-utils'
import NotificationHistory from './NotificationHistory.vue'
import NotificationHistorySettings from './NotificationHistorySettings.vue'

const mocks = vi.hoisted(() => ({history: vi.fn(), read: vi.fn(), write: vi.fn(), error: vi.fn(), success: vi.fn()}))
vi.mock('@/client/generated', () => ({tasktraceNotificationHistoryList: mocks.history, tasktraceNotificationSettingsRead: mocks.read, tasktraceNotificationSettingsWrite: mocks.write}))
vi.mock('@/helpers/tasktraceLocal', () => ({isLocalBuild: true}))
vi.mock('@/stores/auth', () => ({useAuthStore: () => ({info: {id: 1, username: 'me'}})}))
vi.mock('@/message', () => ({getErrorText: (cause: Error) => cause.message, error: mocks.error, success: mocks.success}))
const stubs = {Card: {template: '<section><slot /></section>'}, XButton: {template: '<button><slot /></button>'}, TeamMemberIdentity: {props: ['username'], template: '<span>{{ username }}</span>'}}
let wrapper: VueWrapper | undefined
beforeEach(() => {vi.resetAllMocks()})
afterEach(() => {wrapper?.unmount(); wrapper = undefined})

describe('notification history', () => {
 it('keeps the reading list stable and emits archived collaboration targets', async () => {
  const team = {id: 'archived', actor: 'Alice', task_title: '修复图片', task_id: 12, comment_id: 34, read: true}
  mocks.history.mockResolvedValue({data: [{id: 'team:share:archived', archived_at: '2026-10-13T08:00:00Z', team}]})
  wrapper = mount(NotificationHistory, {global: {stubs}}); await flushPromises()
  expect(wrapper.text()).toContain('修复图片')
  const list = wrapper.get('.history-list').element as HTMLElement
  list.scrollTop = 120
  window.dispatchEvent(new Event('focus')); await flushPromises()
  expect(list.scrollTop).toBe(120)
  expect(mocks.history).toHaveBeenCalledTimes(1)
  await wrapper.get('.history-link').trigger('click')
  expect(wrapper.emitted('openTeam')?.[0]).toEqual([team])
 })
 it('renders ordinary notifications and emits the original task target', async () => {
  mocks.history.mockResolvedValue({data: [{id: 'notification:1', notification: {id: 1, name: 'task.comment', created: '2026-10-13T08:00:00Z', notification: {doer: {id: 2, name: 'Alice'}, task: {id: 12, title: 'Task'}, comment: {id: 34}}}}]})
  wrapper = mount(NotificationHistory, {global: {stubs}}); await flushPromises()
  expect(wrapper.text()).toContain('Alice')
  await wrapper.get('.history-link').trigger('click')
  expect((wrapper.emitted('openNotification')?.[0][0] as any).notification.task.id).toBe(12)
 })
 it('offers retry on load failure and shows the empty state after recovery', async () => {
  mocks.history.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce({data: []})
  wrapper = mount(NotificationHistory, {global: {stubs}}); await flushPromises()
  expect(wrapper.get('[role="alert"]').text()).toContain('offline')
  await wrapper.get('button').trigger('click'); await flushPromises()
  expect(wrapper.text()).toContain('暂无历史通知')
 })
})

describe('notification history settings', () => {
 it('loads default 100 and saves a new limit', async () => {
  mocks.read.mockResolvedValue({data: {max_entries: 100}})
  mocks.write.mockResolvedValue({data: {max_entries: 25}})
  wrapper = mount(NotificationHistorySettings, {global: {stubs}}); await flushPromises()
  expect((wrapper.get('input').element as HTMLInputElement).value).toBe('100')
  await wrapper.get('input').setValue('25'); await wrapper.get('button').trigger('click'); await flushPromises()
  expect(mocks.write).toHaveBeenCalledWith({body: {max_entries: 25}})
  expect(mocks.success).toHaveBeenCalledTimes(1)
 })
 it.each(['', '0', '10001', '1.5'])('rejects invalid retention %s without a request', async value => {
  mocks.read.mockResolvedValue({data: {max_entries: 100}})
  wrapper = mount(NotificationHistorySettings, {global: {stubs}}); await flushPromises()
  await wrapper.get('input').setValue(value); await wrapper.get('button').trigger('click'); await flushPromises()
  expect(mocks.write).not.toHaveBeenCalled()
  expect(mocks.error).toHaveBeenCalledTimes(1)
 })
 it('never overwrites settings with defaults after a failed read', async () => {
  mocks.read.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce({data: {max_entries: 12}})
  wrapper = mount(NotificationHistorySettings, {global: {stubs}}); await flushPromises()
  expect(wrapper.get('button').text()).toBe('重新读取设置')
  await wrapper.get('button').trigger('click'); await flushPromises()
  expect(mocks.write).not.toHaveBeenCalled()
  expect((wrapper.get('input').element as HTMLInputElement).value).toBe('12')
 })
})
