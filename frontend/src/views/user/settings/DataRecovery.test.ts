import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest'
import {flushPromises, mount, type VueWrapper} from '@vue/test-utils'
import DataRecovery from './DataRecovery.vue'

const sdk = vi.hoisted(() => ({
	tasktraceDataRecoveryDetect: vi.fn(),
	tasktraceDataRecoveryImport: vi.fn(),
	tasktraceDataBackupRun: vi.fn(),
	tasktraceDataBackupSettingsRead: vi.fn(),
	tasktraceDataBackupSettingsWrite: vi.fn(),
	tasktraceDataBackupStatus: vi.fn(),
}))
const messages = vi.hoisted(() => ({error: vi.fn(), success: vi.fn()}))
vi.mock('@/client/generated', () => sdk)
vi.mock('@/message', () => messages)
vi.mock('@/composables/useTitle', () => ({useTitle: vi.fn()}))

let wrapper: VueWrapper | undefined

beforeEach(() => {
	vi.resetAllMocks()
	sdk.tasktraceDataRecoveryDetect.mockResolvedValue({data: {candidates: []}})
	sdk.tasktraceDataBackupSettingsRead.mockResolvedValue({data: {
		enabled: true,
		directory: 'backups',
		daily_time: '02:00',
		retention_days: 30,
		minimum_backups: 3,
	}})
	sdk.tasktraceDataBackupSettingsWrite.mockImplementation(async ({body}) => ({data: {...body}}))
	sdk.tasktraceDataBackupStatus.mockResolvedValue({data: {backup_count: 0}})
	sdk.tasktraceDataBackupRun.mockResolvedValue({data: {created: true, message: '备份已完成。'}})
})

afterEach(() => {
	wrapper?.unmount()
	wrapper = undefined
})

async function mountSettings() {
	wrapper = mount(DataRecovery, {
		global: {
			stubs: {
				Card: {template: '<section><slot /></section>'},
				FormField: {template: '<label><slot /></label>'},
				FormCheckbox: true,
				Modal: true,
				XButton: {
					template: '<button type="button" @click="$emit(\'click\', $event)"><slot /></button>',
					emits: ['click'],
				},
			},
		},
	})
	await flushPromises()
	return wrapper
}

async function clickAction(page: VueWrapper, label: string) {
	const button = page.findAll('button').find(item => item.text() === label)
	expect(button).toBeDefined()
	await button!.trigger('click')
	await flushPromises()
}

describe('data recovery backup time', () => {
	it('saves the loaded daily backup time', async () => {
		const page = await mountSettings()
		expect(page.get<HTMLInputElement>('input[type="time"]').element.value).toBe('02:00')
		await clickAction(page, '保存备份设置')
		expect(sdk.tasktraceDataBackupSettingsWrite).toHaveBeenCalledExactlyOnceWith({body: {
			enabled: true, directory: 'backups', daily_time: '02:00', retention_days: 30, minimum_backups: 3,
		}})
		expect(messages.error).not.toHaveBeenCalled()
	})

	it.each(['00:00', '23:59'])('saves the edited boundary time %s', async dailyTime => {
		const page = await mountSettings()
		await page.get('input[type="time"]').setValue(dailyTime)
		await clickAction(page, '保存备份设置')
		expect(sdk.tasktraceDataBackupSettingsWrite).toHaveBeenCalledWith({body: expect.objectContaining({daily_time: dailyTime})})
		expect(messages.error).not.toHaveBeenCalled()
	})

	it('runs a manual backup after saving the chosen time', async () => {
		const page = await mountSettings()
		await page.get('input[type="time"]').setValue('18:35')
		await clickAction(page, '立即备份')
		expect(sdk.tasktraceDataBackupSettingsWrite).toHaveBeenCalledWith({body: expect.objectContaining({daily_time: '18:35'})})
		expect(sdk.tasktraceDataBackupRun).toHaveBeenCalledOnce()
		expect(sdk.tasktraceDataBackupSettingsWrite.mock.invocationCallOrder[0]).toBeLessThan(sdk.tasktraceDataBackupRun.mock.invocationCallOrder[0])
		expect(messages.error).not.toHaveBeenCalled()
		expect(messages.success).toHaveBeenCalledWith({message: '备份已完成。'})
	})

	it.each(['保存备份设置', '立即备份'])('still rejects a missing time when clicking %s', async label => {
		const page = await mountSettings()
		await page.get('input[type="time"]').setValue('')
		await clickAction(page, label)
		expect(sdk.tasktraceDataBackupSettingsWrite).not.toHaveBeenCalled()
		expect(sdk.tasktraceDataBackupRun).not.toHaveBeenCalled()
		expect(messages.error).toHaveBeenCalledWith(expect.objectContaining({message: '请选择每天备份的时间。'}))
	})
})
