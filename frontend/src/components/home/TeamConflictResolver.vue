<template>
	<Modal
		:enabled="true"
		:wide="true"
		aria-label="逐项处理协作冲突"
		@close="$emit('close')"
	>
		<section class="conflict-dialog">
			<header class="conflict-header">
				<div>
					<h2>处理协作冲突</h2>
					<p>对比各个版本，每次只保存当前这一项。其余冲突可稍后继续处理。</p>
				</div>
				<XButton
					variant="secondary"
					:wrap="false"
					@click="$emit('close')"
				>
					关闭
				</XButton>
			</header>
			<div
				v-if="conflicts.length"
				class="conflict-layout"
			>
				<nav
					class="conflict-list"
					aria-label="待处理冲突"
				>
					<h3>待处理 {{ conflicts.length }} 项</h3>
					<button
						v-for="(item, index) in conflicts"
						:key="item.id"
						type="button"
						:aria-current="item.id === activeId ? 'true' : undefined"
						:disabled="saving"
						@click="activeId = item.id || ''"
					>
						<span>{{ index + 1 }}. {{ item.task_title || '未命名任务' }}</span>
						<small>{{ conflictFieldLabel(item.field) }}</small>
					</button>
				</nav>
				<div
					v-if="active"
					class="conflict-review"
				>
					<div class="conflict-context">
						<h3>{{ active.task_title }} · {{ conflictFieldLabel(active.field) }}</h3>
						<p>高亮处与上次同步不同。选择一个完整版本，然后保存这一项。</p>
					</div>
					<fieldset
						class="conflict-versions"
						:disabled="saving"
					>
						<legend class="is-sr-only">
							选择此冲突要保留的版本
						</legend>
						<section
							v-for="version in versions"
							:key="version.key"
							class="conflict-version"
							:class="{'is-selected': selected === version.key}"
						>
							<label class="version-heading">
								<input
									v-model="selected"
									type="radio"
									:name="`conflict-${active.id}`"
									:value="version.key"
								>
								<span><strong>{{ version.label }}</strong><span v-if="version.author">{{ version.author.split('、').map(teamStore.displayNameFor).join('、') }}</span></span>
							</label>
							<dl class="version-details">
								<div
									v-for="detail in version.details"
									:key="detail.key"
									class="version-field"
									:class="{'is-changed': isChanged(version.key, detail)}"
								>
									<dt>{{ detail.label }}<span v-if="isChanged(version.key, detail)">有差异</span></dt>
									<dd>
										<template
											v-for="(part, index) in segments(version.key, detail)"
											:key="index"
										>
											<mark v-if="part.changed">{{ part.text }}</mark><span v-else>{{ part.text }}</span>
										</template>
									</dd>
									<dd
										v-if="detail.images"
										class="version-images"
									>
										<ReadonlyRichText :html="detail.images" />
									</dd>
								</div>
							</dl>
						</section>
					</fieldset>
					<p
						v-if="selectedVersion?.value === '' && active.field?.startsWith('outstanding')"
						class="conflict-delete-hint"
					>
						所选版本不保留此遗留事项，保存后该条目将被移除。
					</p>
					<p
						v-if="saveError"
						class="conflict-error"
						role="alert"
					>
						{{ saveError }}
					</p>
					<footer class="conflict-footer">
						<span role="status">{{ resultMessage || '尚未保存当前选择' }}</span>
						<XButton
							variant="secondary"
							:disabled="saving || conflicts.length < 2"
							@click="skip"
						>
							稍后处理此项
						</XButton>
						<XButton
							variant="primary"
							:disabled="saving || !selectedVersion"
							@click="save"
						>
							{{ saving ? '正在保存…' : '保存此项并继续' }}
						</XButton>
					</footer>
				</div>
			</div>
			<div
				v-else
				class="conflict-empty"
				role="status"
			>
				<h3>{{ resolvedCount ? '当前冲突已全部处理' : '当前没有待处理的冲突' }}</h3>
				<p>{{ resolvedCount ? `本次已处理 ${resolvedCount} 项，结果已保存到团队仓库。` : '成员更新通知可在顶部通知铃铛中查看。' }}</p>
			</div>
		</section>
	</Modal>
</template>

<script setup lang="ts">
import {computed, ref, watch} from 'vue'
import Modal from '@/components/misc/Modal.vue'
import XButton from '@/components/input/Button.vue'
import ReadonlyRichText from '@/components/tasks/partials/ReadonlyRichText.vue'
import {useTasktraceTeamStore} from '@/stores/tasktraceTeam'
import {getErrorText} from '@/message'
import {changedText, conflictFieldLabel, conflictVersions, type ConflictDetail} from '@/helpers/teamConflictDiff'

defineEmits<{close: []}>()
const teamStore = useTasktraceTeamStore()
const conflicts = computed(() => teamStore.status.conflicts || [])
const activeId = ref('')
const active = computed(() => conflicts.value.find(item => item.id === activeId.value))
const versions = computed(() => active.value ? conflictVersions(active.value) : [])
const selected = ref('')
const selectedVersion = computed(() => versions.value.find(version => version.key === selected.value))
const saving = ref(false)
const saveError = ref('')
const resultMessage = ref('')
const resolvedCount = ref(0)
watch(() => conflicts.value.map(item => item.id), (ids, previous) => {
	if (!ids.includes(activeId.value)) {
		const index = Math.max(0, previous?.indexOf(activeId.value) || 0)
		activeId.value = ids[Math.min(index, ids.length - 1)] || ''
	}
}, {immediate: true})
watch(activeId, () => { selected.value = ''; saveError.value = '' })
function isChanged(key: string, detail: ConflictDetail) {
	return key !== 'base' && versions.value[0]?.details.find(base => base.key === detail.key)?.comparison !== detail.comparison
}
function segments(key: string, detail: ConflictDetail) {
	const base = versions.value[0]?.details.find(item => item.key === detail.key)
	return key === 'base' ? [{text: detail.text, changed: false}] : changedText(base?.text || '', detail.text)
}
function skip() {
	const index = conflicts.value.findIndex(item => item.id === activeId.value)
	activeId.value = conflicts.value[(index + 1) % conflicts.value.length]?.id || ''
	resultMessage.value = '已跳过，未修改这一项'
}
async function save() {
	const conflict = active.value, version = selectedVersion.value
	if (!conflict?.id || !conflict.share_id || !version || saving.value) return
	saving.value = true
	saveError.value = ''
	try {
		// Revalidate the selected immutable conflict ID before applying it.
		await teamStore.refresh(true)
		if (!conflicts.value.some(item => item.id === conflict.id)) {
			resultMessage.value = '这一项已被更新或处理，请重新查看当前差异。'
			return
		}
		await teamStore.resolve(conflict.share_id, [{conflict_id: conflict.id, value: version.value}])
		if (conflicts.value.some(item => item.id === conflict.id)) throw new Error('此项仍未处理，请检查共享目录连接后重试。')
		resolvedCount.value++
		resultMessage.value = `已保存 ${resolvedCount.value} 项，其他未选中的冲突保持不变`
	} catch (cause) { saveError.value = getErrorText(cause) }
	finally { saving.value = false }
}
</script>

<style scoped lang="scss">
.conflict-dialog {
	background: var(--white);
	color: var(--text);
	text-align: start;
	border-radius: .75rem;
	overflow: hidden;
}
.conflict-header {
	display: flex;
	align-items: flex-start;
	justify-content: space-between;
	gap: 1rem;
	padding: 1.25rem;
	border-block-end: 1px solid var(--grey-200);
}
.conflict-header > .button {
	flex-shrink: 0;
}
.conflict-header h2 {
	font-size: 1.35rem;
	font-weight: 700;
	margin: 0 0 .35rem;
	color: var(--text-strong);
}
.conflict-header p, .conflict-context p {
	margin: 0;
	color: var(--grey-600);
	font-size: .875rem;
}
.conflict-layout {
	display: grid;
	grid-template-columns: 12rem minmax(0, 1fr);
}
.conflict-list {
	padding: 1rem .75rem;
	background: var(--grey-50);
	border-inline-end: 1px solid var(--grey-200);
	max-block-size: 65dvh;
	overflow-y: auto;
}
.conflict-list h3 {
	font-size: .875rem;
	margin: 0 0 .75rem;
}
.conflict-list button {
	display: grid;
	gap: .3rem;
	inline-size: 100%;
	padding: .7rem;
	margin-block-end: .3rem;
	border: 1px solid transparent;
	border-radius: .375rem;
	text-align: start;
	color: var(--text);
	background: transparent;
	font: inherit;
	cursor: pointer;
	overflow-wrap: anywhere;
}
.conflict-list button[aria-current] {
	border-color: var(--primary);
	background: var(--white);
}
.conflict-list button:hover {
	background: var(--grey-100);
}
.conflict-list small {
	color: var(--grey-600);
}
.conflict-review {
	padding: 1.25rem;
	min-inline-size: 0;
}
.conflict-context h3 {
	margin: 0 0 .4rem;
	font-size: 1.05rem;
	font-weight: 650;
	overflow-wrap: anywhere;
}
.conflict-versions {
	display: grid;
	grid-template-columns: repeat(auto-fit, minmax(min(100%, 13rem), 1fr));
	gap: .75rem;
	padding: 0;
	margin: 1rem 0;
	border: 0;
}
.conflict-version {
	border: 1px solid var(--grey-300);
	border-radius: .5rem;
	overflow: hidden;
	min-inline-size: 0;
}
.conflict-version.is-selected {
	border-color: var(--primary);
}
.version-heading {
	min-block-size: 4.75rem;
	display: flex;
	gap: .65rem;
	align-items: flex-start;
	padding: .85rem;
	background: var(--grey-50);
	cursor: pointer;
}
.version-heading > span {
	display: grid;
	gap: .3rem;
	min-inline-size: 0;
	overflow-wrap: anywhere;
}
.version-heading input {
	margin-block-start: .25rem;
	accent-color: var(--primary);
}
.version-heading span span {
	font-size: .8rem;
	color: var(--grey-600);
}
.version-details {
	margin: 0;
}
.version-field {
	padding: .75rem .85rem;
	border-block-start: 1px solid var(--grey-200);
}
.version-field dt {
	display: flex;
	justify-content: space-between;
	gap: .5rem;
	font-size: .75rem;
	font-weight: 650;
	color: var(--grey-600);
	margin-block-end: .4rem;
}
.version-field dt span {
	color: var(--text-strong);
}
.version-field dd {
	margin: 0;
	white-space: pre-wrap;
	overflow-wrap: anywhere;
	font-size: .9rem;
	line-height: 1.65;
}
.version-field.is-changed {
	background: var(--grey-50);
}
.version-field mark {
	background: #fff0bd;
	color: #543600;
	border-radius: .15rem;
}
.version-images :deep(img) {
	max-block-size: 10rem;
	margin-block-start: .5rem;
}
.conflict-footer {
	display: flex;
	align-items: center;
	flex-wrap: wrap;
	gap: .65rem;
	padding-block-start: 1rem;
	border-block-start: 1px solid var(--grey-200);
}
.conflict-footer > span {
	flex: 1 1 100%;
	font-size: .8rem;
	color: var(--grey-600);
}
.conflict-error, .conflict-delete-hint {
	color: var(--danger);
	overflow-wrap: anywhere;
	margin: .75rem 0;
}
.conflict-empty {
	padding: 3rem 1.5rem;
	text-align: center;
}
.conflict-empty h3 {
	font-size: 1.2rem;
	margin-block-end: .5rem;
}
.conflict-empty p {
	color: var(--grey-600);
}
@media (width <= 700px) {
	.conflict-layout {
		grid-template-columns: minmax(0, 1fr);
	}
	.conflict-list {
		max-block-size: 12rem;
		border-inline-end: 0;
		border-block-end: 1px solid var(--grey-200);
	}
	.conflict-header, .conflict-review {
		padding: 1rem;
	}
}
</style>
