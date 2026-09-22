import {beforeEach, describe, expect, it, vi} from 'vitest'
import {createPinia, setActivePinia} from 'pinia'
import {watch} from 'vue'
import {tasktraceUpdateStatus, tasktraceUpdateIgnore} from '@/client/generated'
import {useTasktraceUpdateStore} from './tasktraceUpdate'

vi.mock('@/client/generated', async original => ({
	...(await original<typeof import('@/client/generated')>()),
	tasktraceUpdateStatus: vi.fn(), tasktraceUpdateIgnore: vi.fn(),
}))
const snapshot = () => ({status: 'idle', available: true, latest_version: 'v1.0.0', notify: true})
beforeEach(() => {
	setActivePinia(createPinia())
	vi.resetAllMocks()
	vi.mocked(tasktraceUpdateStatus).mockImplementation(async () => ({data: snapshot()}) as never)
})
describe('update status refresh', () => {
	it('defers a poll started while an ignore request is saving', async () => {
		const store = useTasktraceUpdateStore()
		await store.refresh()
		vi.mocked(tasktraceUpdateStatus).mockClear()
		let finish!: (value: never) => void
		vi.mocked(tasktraceUpdateIgnore).mockImplementationOnce(() => new Promise<never>(resolve => { finish = resolve }))
		const save = store.ignore()
		const read = store.refresh()
		await Promise.resolve()
		expect(tasktraceUpdateStatus).not.toHaveBeenCalled()
		const next = {...snapshot(), notify: false}
		vi.mocked(tasktraceUpdateStatus).mockResolvedValue({data: next} as never)
		finish({data: next} as never)
		await Promise.all([save, read])
		expect(store.shouldNotify).toBe(false)
		expect(tasktraceUpdateStatus).toHaveBeenCalledTimes(1)
	})
	it('coalesces concurrent readers', async () => {
		const store = useTasktraceUpdateStore()
		await Promise.all(Array.from({length: 20}, () => store.refresh()))
		expect(tasktraceUpdateStatus).toHaveBeenCalledTimes(1)
	})
	it('does not invalidate the notification view for identical status', async () => {
		const store = useTasktraceUpdateStore()
		await store.refresh()
		const changed = vi.fn()
		const stop = watch(() => store.state, changed, {flush: 'sync'})
		for (let i = 0; i < 20; i++) await store.refresh()
		stop()
		expect(changed).not.toHaveBeenCalled()
	})
	it('does not re-enable an ignored release from a stale read', async () => {
		const store = useTasktraceUpdateStore()
		await store.refresh()
		let finish!: (value: never) => void
		vi.mocked(tasktraceUpdateStatus).mockImplementationOnce(() => new Promise<never>(resolve => { finish = resolve }))
		const pending = store.refresh()
		vi.mocked(tasktraceUpdateIgnore).mockResolvedValue({data: {...snapshot(), notify: false}} as never)
		await store.ignore()
		finish({data: snapshot()} as never)
		await pending
		expect(store.shouldNotify).toBe(false)
	})
})
