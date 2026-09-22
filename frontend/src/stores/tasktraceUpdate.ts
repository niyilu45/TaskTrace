import {computed, ref} from 'vue'
import {defineStore} from 'pinia'
import equal from 'fast-deep-equal'
import {
	tasktraceUpdateCheck,
	tasktraceUpdateIgnore,
	tasktraceUpdateInstall,
	tasktraceUpdateSettingsRead,
	tasktraceUpdateSettingsWrite,
	tasktraceUpdateStatus,
	type Settings,
	type State,
} from '@/client/generated'

const wait = (milliseconds: number) => new Promise(resolve => setTimeout(resolve, milliseconds))

export const useTasktraceUpdateStore = defineStore('tasktrace-update', () => {
	const state = ref<State>({status: 'idle'})
	const settings = ref<Settings>({check_interval_minutes: 60, ignored_version: ''})
	const available = computed(() => Boolean(state.value.available && state.value.latest_version))
	const shouldNotify = computed(() => Boolean(available.value && state.value.notify))
	const checking = ref(false)
	const supported = ref(true)
	let activeRead: Promise<void> | undefined
	let revision = 0
	const mutations = new Set<Promise<unknown>>()

	function refresh(): Promise<void> {
		if (mutations.size) return Promise.allSettled([...mutations]).then(() => refresh())
		if (activeRead) return activeRead
		const version = revision
		const operation = readStatus(version).finally(() => { if (activeRead === operation) activeRead = undefined })
		activeRead = operation
		return operation
	}

	function mutate<T>(write: () => Promise<T>): Promise<T> {
		revision++
		activeRead = undefined
		const operation = Promise.resolve().then(write).finally(() => {
			mutations.delete(operation)
			revision++
			activeRead = undefined
		})
		mutations.add(operation)
		return operation
	}

	async function readStatus(version: number) {
		try {
			const {data} = await tasktraceUpdateStatus()
			if (version !== revision) return
			if (!equal(state.value, data)) state.value = data
			supported.value = true
		} catch (cause) {
			if (version !== revision) return
			const status = (cause as {status?: number}).status
			if (status === 404) supported.value = false
			else throw cause
		}
	}

	async function loadSettings() {
		const {data} = await tasktraceUpdateSettingsRead()
		settings.value = data
		return data
	}

	async function saveSettings(intervalMinutes: number) {
		const {data} = await tasktraceUpdateSettingsWrite({body: {
			check_interval_minutes: intervalMinutes,
			ignored_version: settings.value.ignored_version || '',
		}})
		settings.value = data
		return data
	}

	async function checkNow() {
		if (checking.value) return state.value
		checking.value = true
		const previousCheck = state.value.checked_at
		try {
			await mutate(() => tasktraceUpdateCheck())
			let observedCurrentCheck = false
			for (let attempt = 0; attempt < 90; attempt++) {
				await wait(500)
				await refresh()
				if (state.value.status === 'queued' || state.value.status === 'checking') observedCurrentCheck = true
				const completedCurrentCheck = Boolean(state.value.checked_at && state.value.checked_at !== previousCheck)
				if (state.value.status === 'error' && (observedCurrentCheck || completedCurrentCheck)) throw new Error(state.value.error || '检查更新失败。')
				if (completedCurrentCheck && state.value.status === 'idle') return state.value
			}
			throw new Error('检查更新超时，请确认系统托盘中的 TaskTrace 正在运行。')
		} finally {
			checking.value = false
		}
	}

	async function ignore() {
		const version = state.value.latest_version
		if (!version) return
		await mutate(async () => {
			const {data} = await tasktraceUpdateIgnore({body: {version}})
			state.value = data
			settings.value.ignored_version = version
		})
	}

	async function install() {
		const version = state.value.latest_version
		if (!version) return
		await mutate(async () => {
			await tasktraceUpdateInstall({body: {version}})
			state.value = {...state.value, status: 'downloading'}
		})
	}

	return {state, settings, available, shouldNotify, checking, supported, refresh, loadSettings, saveSettings, checkNow, ignore, install}
})
