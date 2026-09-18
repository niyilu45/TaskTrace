<template>
	<Card
		title="更新设置"
		:loading="loading"
	>
		<p class="mbe-4">
			TaskTrace 会在启动时检查 GitHub Releases，并按下面的间隔自动检查。下载更新时使用 Windows 系统代理。
		</p>
		<FormField
			label="自动检测间隔（分钟）"
			layout="two-col"
		>
			<FormInput
				v-model.number="intervalMinutes"
				type="number"
				:min="5"
				:max="10080"
				:step="5"
			/>
		</FormField>
		<p class="help mbe-4">
			最短 5 分钟，最长 7 天；默认 60 分钟。
		</p>
		<div class="update-actions">
			<XButton
				:loading="saving"
				@click="save"
			>
				保存更新设置
			</XButton>
			<XButton
				variant="secondary"
				:loading="updateStore.checking"
				@click="check"
			>
				检查更新
			</XButton>
		</div>
	</Card>

	<Card
		title="版本信息"
		class="mts-4"
	>
		<dl class="version-details">
			<div><dt>当前版本</dt><dd>{{ updateStore.state.current_version || '未知' }}</dd></div>
			<div><dt>最近检查</dt><dd>{{ displayDate(updateStore.state.checked_at) }}</dd></div>
			<div v-if="updateStore.state.error">
				<dt>检测结果</dt><dd class="has-text-danger">
					{{ updateStore.state.error }}
				</dd>
			</div>
			<div v-else>
				<dt>检测结果</dt><dd>{{ updateStore.available ? `发现 ${updateStore.state.latest_version}` : '当前已是最新版本' }}</dd>
			</div>
		</dl>
	</Card>

	<Modal
		:enabled="showUpdateModal"
		@close="declineUpdate"
		@submit="installUpdate"
	>
		<template #header>
			发现新版本 {{ updateStore.state.latest_version }}
		</template>
		<template #text>
			<p><strong>发布日期：</strong>{{ displayDate(updateStore.state.published_at) }}</p>
			<p class="mbs-3">
				<strong>更新内容：</strong>
			</p>
			<pre class="release-notes">{{ updateStore.state.release_notes || '本次发布未填写更新内容。' }}</pre>
			<p class="mbs-4">
				更新需要关闭正在运行的 TaskTrace。确认后将下载免安装包、关闭程序、替换文件并自动重新启动。
			</p>
		</template>
	</Modal>
</template>

<script setup lang="ts">
import {onMounted, ref} from 'vue'
import FormField from '@/components/input/FormField.vue'
import FormInput from '@/components/input/FormInput.vue'
import XButton from '@/components/input/Button.vue'
import Modal from '@/components/misc/Modal.vue'
import {error as showError, success} from '@/message'
import {useTasktraceUpdateStore} from '@/stores/tasktraceUpdate'
import {useTitle} from '@/composables/useTitle'

defineOptions({name: 'TaskTraceUpdateSettings'})
useTitle(() => '更新设置 - TaskTrace')
const updateStore = useTasktraceUpdateStore()
const intervalMinutes = ref(60)
const loading = ref(true)
const saving = ref(false)
const showUpdateModal = ref(false)

onMounted(async () => {
	try {
		const settings = await updateStore.loadSettings()
		intervalMinutes.value = settings.check_interval_minutes || 60
		await updateStore.refresh()
	} catch (cause) {
		showError(cause)
	} finally {
		loading.value = false
	}
})

async function save() {
	if (!Number.isInteger(intervalMinutes.value) || intervalMinutes.value < 5 || intervalMinutes.value > 10080) {
		showError(new Error('检测间隔必须是 5 到 10080 之间的整数分钟。'))
		return
	}
	saving.value = true
	try {
		await updateStore.saveSettings(intervalMinutes.value)
		success({message: '更新设置已保存。'})
	} catch (cause) {
		showError(cause)
	} finally {
		saving.value = false
	}
}

async function check() {
	try {
		await updateStore.checkNow()
		if (updateStore.available) showUpdateModal.value = true
		else success({message: '当前已是最新版本。'})
	} catch (cause) {
		showError(cause)
	}
}

async function declineUpdate() {
	showUpdateModal.value = false
	try { await updateStore.ignore() } catch (cause) { showError(cause) }
}

async function installUpdate() {
	try {
		await updateStore.install()
		showUpdateModal.value = false
		success({message: '正在使用系统代理下载更新，完成后 TaskTrace 将关闭并重新启动。'})
	} catch (cause) {
		showError(cause)
	}
}

function displayDate(value?: string) {
	if (!value) return '尚未检查'
	const date = new Date(value)
	return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}
</script>

<style lang="scss" scoped>
.update-actions {
	display: flex;
	flex-wrap: wrap;
	gap: .75rem;
}

.version-details {
	display: grid;
	gap: .75rem;

	div {
		display: grid;
		grid-template-columns: minmax(6rem, 9rem) 1fr;
		gap: 1rem;
	}

	dt { color: var(--grey-500); }
	dd {
		margin: 0;
		overflow-wrap: anywhere;
	}
}

.release-notes {
	max-block-size: 16rem;
	overflow: auto;
	white-space: pre-wrap;
	font: inherit;
	background: var(--grey-100);
	border-radius: $radius;
	padding: .75rem;
}
</style>
