<template>
	<Card
		title="定时备份"
		:loading="backupLoading"
	>
		<p class="mbe-4">
			每天在指定时间备份个人数据和团队协作数据。备份前会比较完整内容，数据没有变化时不会新增备份。
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
				label="每天备份时间"
				layout="two-col"
			>
				<FormInput
					v-model="backupSettings.daily_time"
					type="time"
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
				立即备份
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
				<dt>下次定时备份</dt><dd>{{ formatDate(backupStatus.next_check_at) }}</dd>
			</div>
			<div v-if="backupStatus.last_message">
				<dt>最近结果</dt><dd>{{ backupStatus.last_message }}</dd>
			</div>
		</dl>
	</Card>

	<Card
		title="从备份恢复"
		:loading="backupDetecting"
		class="mts-4"
	>
		<p>从已配置的备份目录选择一份历史备份。恢复内容会复制到新的受保护目录，当前数据不会被覆盖。</p>
		<XButton
			class="mbs-4"
			:loading="backupDetecting"
			@click="detectBackups"
		>
			查看可恢复备份
		</XButton>
		<p v-if="backupDetection && backupCandidates.length === 0">
			当前备份目录中没有可恢复的数据。
		</p>
		<div
			v-else-if="backupCandidates.length > 0"
			class="candidate-list mbs-4"
		>
			<article
				v-for="candidate in backupCandidates"
				:key="candidate.data_directory"
				class="candidate"
			>
				<div class="candidate__details">
					<strong>{{ formatDate(candidate.modified_at) }}</strong>
					<span>事项 {{ candidate.tasks || 0 }} · 项目 {{ candidate.projects || 0 }} · 用户 {{ candidate.users || 0 }}</span>
					<span>{{ candidate.data_directory }}</span>
				</div>
				<XButton
					variant="secondary"
					@click="selectCandidate(candidate, 'backup')"
				>
					恢复此备份
				</XButton>
			</article>
		</div>
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
			label="数据目录或程序目录"
			layout="two-col"
			class="mbs-4"
		>
			<FormInput
				v-model="manualPath"
				placeholder="例如 D:\TaskTrace数据 或 D:\TaskTrace数据\data"
			/>
		</FormField>
		<FormField
			label="配套 teamData 目录（可选）"
			layout="two-col"
			class="mbs-3"
		>
			<FormInput
				v-model="manualTeamPath"
				placeholder="data 与 teamData 分开存放时，填写团队数据路径"
			/>
		</FormField>
		<p class="help mbe-4">
			支持新版和旧版数据。可以粘贴单独复制的 data 文件夹或其上一级目录，也会检测其中的 imports 数据子目录。
		</p>
		<p class="help mbe-4">
			配套的 teamData 放在 data 旁边时会自动匹配；分开存放时请填写上面的团队数据路径。留空检测路径时自动查找当前程序附近的数据。
		</p>
		<XButton
			:loading="detecting"
			@click="detect"
		>
			检测数据
		</XButton>
	</Card>

	<Card
		v-if="detection"
		title="检测结果"
		class="mts-4"
	>
		<p v-if="candidates.length === 0">
			没有检测到可导入的数据。请填写包含 tasktrace.db 的 data 文件夹或其上一级目录后重新检测；当前正在使用的数据不会重复列出。
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
					@click="selectCandidate(candidate, 'data')"
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
			所选数据已复制到新的纯数据目录，当前数据没有被覆盖。请从系统托盘退出 TaskTrace，再重新启动程序。
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
			{{ selectedSource === 'backup' ? '从备份恢复' : '导入数据' }}
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
const backupDetection = ref<TaskTraceDataDetection>()
const selectedCandidate = ref<TaskTraceDataCandidate>()
const selectedSource = ref<'backup' | 'data'>('data')
const imported = ref<TaskTraceDataImportResult>()
const candidates = computed(() => detection.value?.candidates || [])
const backupCandidates = computed(() => backupDetection.value?.candidates || [])

type DailyBackupSettings = TaskTraceBackupSettings & {daily_time?: string}
const backupSettings = ref<DailyBackupSettings>({
	enabled: false,
	directory: 'backups',
	daily_time: '02:00',
	retention_days: 30,
	minimum_backups: 3,
})
const backupStatus = ref<TaskTraceBackupStatus>()
const backupLoading = ref(true)
const backupSaving = ref(false)
const backupRunning = ref(false)
const backupDetecting = ref(false)

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
		backupSettings.value = {...settings, daily_time: (settings as DailyBackupSettings).daily_time || '02:00'}
		backupStatus.value = status
	} catch (cause) {
		showError(cause)
	} finally {
		backupLoading.value = false
	}
}

function validateBackupSettings() {
	const settings = backupSettings.value
	const retentionDays = Number(settings.retention_days)
	const minimumBackups = Number(settings.minimum_backups)
	if (!settings.directory?.trim()) throw new Error('请填写备份路径。')
	if (!/^([01][0-9]|2[0-3]):[0-5][0-9]$/.test(settings.daily_time || '')) throw new Error('请选择每天备份的时间。')
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

async function detectBackups() {
	if (backupDetecting.value) return
	backupDetecting.value = true
	try {
		if (!backupStatus.value?.directory) await loadBackupSettings()
		const path = backupStatus.value?.directory
		if (!path) throw new Error('请先保存备份路径。')
		const {data} = await tasktraceDataRecoveryDetect({query: {path}})
		backupDetection.value = data
	} catch (cause) {
		showError(cause)
	} finally {
		backupDetecting.value = false
	}
}

function selectCandidate(candidate: TaskTraceDataCandidate, source: 'backup' | 'data') {
	selectedSource.value = source
	selectedCandidate.value = candidate
}

async function detect() {
	if (detecting.value) return
	detecting.value = true
	detection.value = undefined
	selectedCandidate.value = undefined
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
			team_data_directory: (selectedSource.value === 'data' ? manualTeamPath.value.trim() : '') || selectedCandidate.value.team_data_directory || '',
		}})
		imported.value = data
		selectedCandidate.value = undefined
		success({message: selectedSource.value === 'backup' ? '备份已安全恢复。退出并重新启动 TaskTrace 后生效。' : '数据已安全复制。退出并重新启动 TaskTrace 后生效。'})
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
