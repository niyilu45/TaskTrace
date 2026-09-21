<template>
	<div class="team-member-picker">
		<label
			class="label"
			:for="inputId"
		>{{ label }}</label>
		<input
			:id="inputId"
			v-model="query"
			class="input"
			type="search"
			autocomplete="off"
			:placeholder="placeholder"
			aria-autocomplete="list"
			:aria-controls="`${inputId}-results`"
			@focus="openPicker"
			@blur="closeLater"
		>
		<div
			v-if="focused"
			:id="`${inputId}-results`"
			class="team-member-results"
			role="listbox"
		>
			<p
				v-if="loading"
				class="team-member-state"
			>
				{{ availableCandidates.length ? '已显示快速匹配，正在继续查找域账户…' : '正在查找 Windows 账户…' }}
			</p>
			<button
				v-for="candidate in availableCandidates"
				:key="candidate.account_name || candidate.username"
				type="button"
				class="team-member-option"
				role="option"
				@mousedown.prevent="choose(candidate)"
			>
				<strong>{{ candidate.display_name || candidate.username }}</strong>
				<span>{{ candidate.account_name }}</span>
			</button>
			<p
				v-if="!loading && !availableCandidates.length"
				class="team-member-state"
			>
				{{ searchError || (query.trim() ? '没有找到可添加的账户' : '还没有已添加的协作人员，请输入姓名或 Windows 用户名继续查找。') }}
			</p>
		</div>
		<p class="help">
			点击输入框会先列出已经添加的协作人员；也可以输入用户名、显示名称或“电脑名\\用户名”继续查找。
		</p>
	</div>
</template>

<script setup lang="ts">
import {computed, onBeforeUnmount, ref, watch} from 'vue'

import type {TaskTraceTeamMemberCandidate} from '@/client/generated'
import {useTasktraceTeamStore} from '@/stores/tasktraceTeam'

const props = withDefaults(defineProps<{
	inputId: string
	label?: string
	placeholder?: string
	excluded?: string[]
}>(), {
	label: '查找协作成员',
	placeholder: '输入姓名或 Windows 用户名',
	excluded: () => [],
})

const emit = defineEmits<{select: [candidate: TaskTraceTeamMemberCandidate]}>()
const teamStore = useTasktraceTeamStore()
const query = ref('')
const candidates = ref<TaskTraceTeamMemberCandidate[]>([])
const loading = ref(false)
const focused = ref(false)
const searchError = ref('')
let searchTimer: ReturnType<typeof setTimeout> | undefined
let searchRevision = 0
let searchController: AbortController | undefined

function memberKey(value = '') {
	return value.trim().split('\\').pop()?.split('@')[0]?.toLowerCase() || ''
}

function mergeCandidates(...groups: TaskTraceTeamMemberCandidate[][]) {
	const merged = new Map<string, TaskTraceTeamMemberCandidate>()
	for (const candidate of groups.flat()) {
		const key = (candidate.account_name || candidate.username || '').trim().toLocaleLowerCase()
		if (key && !merged.has(key)) merged.set(key, candidate)
	}
	return [...merged.values()]
}

const availableCandidates = computed(() => {
	const keyword = query.value.trim().toLocaleLowerCase()
	const combined: TaskTraceTeamMemberCandidate[] = [
		...teamStore.memberRoster
			.filter(member => !keyword || member.toLocaleLowerCase().includes(keyword))
			.map(member => ({username: memberKey(member), account_name: member, display_name: member})),
		...candidates.value,
	]
	const seen = new Set<string>()
	return combined.filter(candidate => {
		const key = memberKey(candidate.account_name || candidate.username)
		if (!key || seen.has(key) || props.excluded.some(member => memberKey(member) === key)) return false
		seen.add(key)
		return true
	})
})

watch(query, value => {
	if (searchTimer) clearTimeout(searchTimer)
	searchController?.abort()
	searchController = undefined
	const revision = ++searchRevision
	const keyword = value.trim()
	if (!keyword) {
		candidates.value = []
		loading.value = false
		searchError.value = ''
		return
	}
	searchTimer = setTimeout(async () => {
		const controller = new AbortController()
		searchController = controller
		let timedOut = false
		const timeout = setTimeout(() => {
			timedOut = true
			controller.abort()
		}, 30_000)
		loading.value = true
		searchError.value = ''
		let quickResult: TaskTraceTeamMemberCandidate[] = []
		try {
			quickResult = await teamStore.searchMembers(keyword, controller.signal, true)
			if (revision === searchRevision) candidates.value = quickResult
			const result = await teamStore.searchMembers(keyword, controller.signal)
			if (revision === searchRevision) candidates.value = mergeCandidates(quickResult, result)
		} catch (cause) {
			if (revision === searchRevision) {
				candidates.value = quickResult
				searchError.value = quickResult.length ? '' : timedOut
					? '查找 Windows 账户超时，请检查域网络或输入更完整的用户名后重试。'
					: cause instanceof Error && cause.message ? cause.message : '无法查询 Windows 账户，请检查本机或域网络后重试。'
			}
		} finally {
			clearTimeout(timeout)
			if (searchController === controller) searchController = undefined
			if (revision === searchRevision) loading.value = false
		}
	}, 250)
})

onBeforeUnmount(() => {
	if (searchTimer) clearTimeout(searchTimer)
	searchController?.abort()
})

function choose(candidate: TaskTraceTeamMemberCandidate) {
	emit('select', candidate)
	query.value = ''
	candidates.value = []
	focused.value = false
}

function openPicker() {
	focused.value = true
	void teamStore.refresh().catch(() => undefined)
}

function closeLater() {
	setTimeout(() => { focused.value = false }, 120)
}
</script>

<style scoped lang="scss">
.team-member-picker {
	position: relative;
}

.team-member-results {
	position: absolute;
	z-index: 20;
	inset: calc(100% - 1.2rem) 0 auto;
	max-block-size: 240px;
	overflow-y: auto;
	border: 1px solid var(--grey-200);
	border-radius: 8px;
	background: var(--white);
	box-shadow: var(--shadow-md);
}

.team-member-option {
	display: flex;
	inline-size: 100%;
	align-items: center;
	justify-content: space-between;
	gap: 1rem;
	padding: .65rem .75rem;
	border: 0;
	border-block-end: 1px solid var(--grey-100);
	background: transparent;
	color: var(--text);
	text-align: start;
	cursor: pointer;
}

.team-member-option:hover,
.team-member-option:focus-visible {
	background: var(--grey-100);
}

.team-member-option span {
	color: var(--grey-500);
	font-size: .8rem;
	overflow-wrap: anywhere;
}

.team-member-state {
	padding: .75rem;
	margin: 0;
	color: var(--grey-500);
}
</style>
