<template>
	<Card
		title="数据保护与恢复"
		:loading="detecting"
	>
		<p>
			TaskTrace 的个人数据和团队数据保存在独立目录中。程序更新包不会包含或覆盖这些目录。
		</p>
		<dl
			v-if="detection"
			class="current-paths mbs-4"
		>
			<div><dt>当前个人数据</dt><dd>{{ detection.current_data_directory }}</dd></div>
			<div><dt>当前团队数据</dt><dd>{{ detection.current_team_data_directory }}</dd></div>
		</dl>

		<FormField
			label="旧程序目录或旧 data 目录"
			layout="two-col"
			class="mbs-4"
		>
			<FormInput
				v-model="manualPath"
				placeholder="例如 D:\旧版TaskTrace 或 D:\旧版TaskTrace\data"
			/>
		</FormField>
		<FormField
			label="旧 teamData 目录（可选）"
			layout="two-col"
			class="mbs-3"
		>
			<FormInput
				v-model="manualTeamPath"
				placeholder="仅在团队数据不位于旧程序旁时填写"
			/>
		</FormField>
		<p class="help mbe-4">
			留空时自动检测当前程序附近的旧版本；也可以粘贴受影响电脑上的完整目录。
		</p>
		<XButton
			:loading="detecting"
			@click="detect"
		>
			检测旧数据
		</XButton>
	</Card>

	<Card
		v-if="detection"
		title="检测结果"
		class="mts-4"
	>
		<p v-if="candidates.length === 0">
			没有检测到可导入的旧数据。请填写旧程序目录或旧 data 目录后重新检测。
		</p>
		<div
			v-else
			class="candidate-list"
		>
			<article
				v-for="candidate in candidates"
				:key="candidate.data_directory || `${candidate.modified_at}-${candidate.database_size}`"
				class="candidate"
			>
				<div class="candidate__details">
					<strong>{{ candidate.data_directory || '未知目录' }}</strong>
					<span>事项 {{ candidate.tasks || 0 }} · 项目 {{ candidate.projects || 0 }} · 用户 {{ candidate.users || 0 }}</span>
					<span>数据库 {{ formatBytes(candidate.database_size) }} · {{ formatDate(candidate.modified_at) }}</span>
					<span v-if="candidate.team_data_directory">团队数据：{{ candidate.team_data_directory }}</span>
					<span v-else>未检测到配套的 teamData</span>
				</div>
				<XButton
					variant="secondary"
					@click="selectedCandidate = candidate"
				>
					导入此数据
				</XButton>
			</article>
		</div>
	</Card>

	<Card
		v-if="imported"
		title="导入准备完成"
		class="mts-4"
	>
		<p>
			旧数据已复制到新的纯数据目录，当前数据没有被覆盖。请从系统托盘退出 TaskTrace，再重新启动程序。
		</p>
		<dl class="current-paths mbs-4">
			<div><dt>个人数据</dt><dd>{{ imported.data_directory }}</dd></div>
			<div><dt>团队数据</dt><dd>{{ imported.team_data_directory }}</dd></div>
			<div v-if="imported.backup_settings">
				<dt>原配置备份</dt><dd>{{ imported.backup_settings }}</dd>
			</div>
		</dl>
	</Card>

	<Modal
		:enabled="Boolean(selectedCandidate)"
		submit-label="复制并导入"
		cancel-label="取消"
		@close="selectedCandidate = undefined"
		@submit="importCandidate"
	>
		<template #header>
			导入旧数据
		</template>
		<template #text>
			<p>程序会把选中的个人数据和团队数据复制到新的纯数据目录，并先备份当前配置。</p>
			<p><strong>不会覆盖或删除当前数据。</strong>导入完成后需要退出并重新启动 TaskTrace。</p>
			<p class="data-path">
				{{ selectedCandidate?.data_directory }}
			</p>
		</template>
	</Modal>
</template>

<script setup lang="ts">
import {computed, onMounted, ref} from 'vue'
import Card from '@/components/misc/Card.vue'
import FormField from '@/components/input/FormField.vue'
import FormInput from '@/components/input/FormInput.vue'
import XButton from '@/components/input/Button.vue'
import Modal from '@/components/misc/Modal.vue'
import {
	tasktraceDataRecoveryDetect,
	tasktraceDataRecoveryImport,
	type TaskTraceDataCandidate,
	type TaskTraceDataDetection,
	type TaskTraceDataImportResult,
} from '@/client/generated'
import {error as showError, success} from '@/message'
import {useTitle} from '@/composables/useTitle'

defineOptions({name: 'TaskTraceDataRecovery'})
useTitle(() => '数据保护与恢复 - TaskTrace')

const manualPath = ref('')
const manualTeamPath = ref('')
const detecting = ref(false)
const importing = ref(false)
const detection = ref<TaskTraceDataDetection>()
const selectedCandidate = ref<TaskTraceDataCandidate>()
const imported = ref<TaskTraceDataImportResult>()
const candidates = computed(() => detection.value?.candidates || [])

onMounted(() => detect())

async function detect() {
	if (detecting.value) return
	detecting.value = true
	try {
		const options = manualPath.value.trim() ? {query: {path: manualPath.value.trim()}} : undefined
		const {data} = await tasktraceDataRecoveryDetect(options)
		detection.value = data
	} catch (cause) {
		showError(cause)
	} finally {
		detecting.value = false
	}
}

async function importCandidate() {
	if (!selectedCandidate.value?.data_directory || importing.value) return
	importing.value = true
	try {
		const {data} = await tasktraceDataRecoveryImport({body: {
			data_directory: selectedCandidate.value.data_directory,
			team_data_directory: manualTeamPath.value.trim() || selectedCandidate.value.team_data_directory || '',
		}})
		imported.value = data
		selectedCandidate.value = undefined
		success({message: '旧数据已安全复制。退出并重新启动 TaskTrace 后生效。'})
	} catch (cause) {
		showError(cause)
	} finally {
		importing.value = false
	}
}

function formatBytes(value?: number) {
	if (typeof value !== 'number' || !Number.isFinite(value) || value <= 0) return '0 B'
	const units = ['B', 'KB', 'MB', 'GB']
	const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1)
	return `${(value / 1024 ** index).toFixed(index === 0 ? 0 : 1)} ${units[index]}`
}

function formatDate(value?: string) {
	if (!value) return '未知时间'
	const date = new Date(value)
	return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}
</script>

<style lang="scss" scoped>
.current-paths {
	display: grid;
	gap: .75rem;

	div {
		display: grid;
		grid-template-columns: minmax(7rem, 10rem) 1fr;
		gap: 1rem;
	}

	dt { color: var(--grey-500); }
	dd {
		margin: 0;
		overflow-wrap: anywhere;
	}
}

.candidate-list {
	display: grid;
	gap: 1rem;
}

.candidate {
	display: flex;
	align-items: center;
	justify-content: space-between;
	gap: 1rem;
	padding: 1rem;
	border: 1px solid var(--grey-200);
	border-radius: var(--card-radius);
	background: var(--white);
}

.candidate__details {
	display: grid;
	gap: .35rem;
	min-inline-size: 0;

	strong,
	span { overflow-wrap: anywhere; }
	span { color: var(--grey-500); }
}

.data-path {
	padding: .75rem;
	border-radius: .5rem;
	background: var(--grey-100);
	overflow-wrap: anywhere;
}

@media (width <= 48rem) {
	.candidate {
		align-items: stretch;
		flex-direction: column;
	}

	.current-paths div {
		grid-template-columns: 1fr;
		gap: .2rem;
	}
}
</style>
