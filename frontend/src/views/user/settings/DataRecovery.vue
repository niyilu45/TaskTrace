<template>
	<Card
		title="定时备份"
		:loading="backupLoading"
	>
		<p class="mbe-4">
			定期备份个人数据和团队协作数据。每次检查会比较完整内容，数据没有变化时不会新增备份。
		</p>
		<FormCheckbox
			v-model="backupSettings.enabled"
			label="启用定时备份"
			class="mbe-4"
		/>
		<FormField
			label="备份路径"
			layout="two-col"
		>
			<FormInput
				v-model="backupSettings.directory"
				placeholder="例如 D:\TaskTrace备份；相对路径默认位于程序目录"
			/>
		</FormField>
		<p class="help mbe-4">
			当前实际路径：{{ backupStatus?.directory || '保存后显示' }}
		</p>
		<div class="backup-grid">
			<FormField
				label="检测间隔（分钟）"
				layout="two-col"
			>
				<FormInput
					v-model.number="backupSettings.interval_minutes"
					type="number"
					:min="1"
					:max="10080"
					:step="1"
				/>
			</FormField>
			<FormField
				label="最多保留天数"
				layout="two-col"
			>
				<FormInput
					v-model.number="backupSettings.retention_days"
					type="number"
					:min="1"
					:max="3650"
					:step="1"
				/>
			</FormField>
			<FormField
				label="最低保留份数"
				layout="two-col"
			>
				<FormInput
					v-model.number="backupSettings.minimum_backups"
					type="number"
					:min="1"
					:max="1000"
					:step="1"
				/>
			</FormField>
		</div>
		<p class="help mbe-4">
			超过保留天数的旧备份会自动清理，但无论多久没有修改，都会保留最新的 {{ backupSettings.minimum_backups || 1 }} 份。
		</p>
		<div class="backup-actions">
			<XButton
				:loading="backupSaving"
				@click="saveBackupSettings"
			>
				保存备份设置
			</XButton>
			<XButton
				variant="secondary"
				:loading="backupRunning"
				@click="runBackupNow"
			>
				立即检查并备份
			</XButton>
		</div>
		<dl
			v-if="backupStatus"
			class="current-paths mbs-4"
		>
			<div><dt>现有备份</dt><dd>{{ backupStatus.backup_count || 0 }} 份</dd></div>
			<div><dt>最近备份</dt><dd>{{ formatDate(backupStatus.last_backup_at) }}</dd></div>
			<div><dt>最近检查</dt><dd>{{ formatDate(backupStatus.last_checked_at) }}</dd></div>
			<div v-if="backupSettings.enabled">
				<dt>下次检查</dt><dd>{{ formatDate(backupStatus.next_check_at) }}</dd>
			</div>
			<div v-if="backupStatus.last_message">
				<dt>最近结果</dt><dd>{{ backupStatus.last_message }}</dd>
			</div>
		</dl>
	</Card>

	<Card
		title="数据保护与恢复"
		:loading="detecting"
		class="mts-4"
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
import FormCheckbox from '@/components/input/FormCheckbox.vue'
import XButton from '@/components/input/Button.vue'
import Modal from '@/components/misc/Modal.vue'
import {
	tasktraceDataRecoveryDetect,
	tasktraceDataRecoveryImport,
	tasktraceDataBackupRun,
	tasktraceDataBackupSettingsRead,
	tasktraceDataBackupSettingsWrite,
	tasktraceDataBackupStatus,
	type TaskTraceBackupSettings,
	type TaskTraceBackupStatus,
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

const backupSettings = ref<TaskTraceBackupSettings>({
	enabled: false,
	directory: 'backups',
	interval_minutes: 60,
	retention_days: 30,
	minimum_backups: 3,
})
const backupStatus = ref<TaskTraceBackupStatus>()
const backupLoading = ref(true)
const backupSaving = ref(false)
const backupRunning = ref(false)

onMounted(() => {
	detect()
	loadBackupSettings()
})

async function loadBackupSettings() {
	backupLoading.value = true
	try {
		const [{data: settings}, {data: status}] = await Promise.all([
			tasktraceDataBackupSettingsRead(),
			tasktraceDataBackupStatus(),
		])
		backupSettings.value = settings
		backupStatus.value = status
	} catch (cause) {
		showError(cause)
	} finally {
		backupLoading.value = false
	}
}

function validateBackupSettings() {
	const settings = backupSettings.value
	const intervalMinutes = Number(settings.interval_minutes)
	const retentionDays = Number(settings.retention_days)
	const minimumBackups = Number(settings.minimum_backups)
	if (!settings.directory?.trim()) throw new Error('请填写备份路径。')
	if (!Number.isInteger(intervalMinutes) || intervalMinutes < 1 || intervalMinutes > 10080) {
		throw new Error('检测间隔必须是 1 到 10080 之间的整数分钟。')
	}
	if (!Number.isInteger(retentionDays) || retentionDays < 1 || retentionDays > 3650) {
		throw new Error('保留天数必须是 1 到 3650 之间的整数。')
	}
	if (!Number.isInteger(minimumBackups) || minimumBackups < 1 || minimumBackups > 1000) {
		throw new Error('最低保留份数必须是 1 到 1000 之间的整数。')
	}
}

async function persistBackupSettings(showMessage = true) {
	validateBackupSettings()
	const {data} = await tasktraceDataBackupSettingsWrite({body: backupSettings.value})
	backupSettings.value = data
	if (showMessage) success({message: '定时备份设置已保存。'})
}

async function saveBackupSettings() {
	if (backupSaving.value) return
	backupSaving.value = true
	try {
		await persistBackupSettings()
		const {data} = await tasktraceDataBackupStatus()
		backupStatus.value = data
	} catch (cause) {
		showError(cause)
	} finally {
		backupSaving.value = false
	}
}

async function runBackupNow() {
	if (backupRunning.value) return
	backupRunning.value = true
	try {
		await persistBackupSettings(false)
		const {data: result} = await tasktraceDataBackupRun()
		const {data: status} = await tasktraceDataBackupStatus()
		backupStatus.value = status
		success({message: result.message || (result.skipped ? '数据没有变化，已跳过本次备份。' : '备份已完成。')})
	} catch (cause) {
		showError(cause)
	} finally {
		backupRunning.value = false
	}
}

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
	if (!value) return '暂无'
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

.backup-grid {
	display: grid;
	gap: .75rem;
}

.backup-actions {
	display: flex;
	flex-wrap: wrap;
	gap: .75rem;
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
