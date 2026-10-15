<template>
	<XButton
		variant="secondary"
		icon="history"
		@click="show"
	>
		周期回顾
	</XButton>
	<Modal
		:enabled="open"
		cancel-label="取消"
		submit-label="查看（新标签页）"
		@close="open = false"
		@submit="view"
	>
		<template #header>
			周期回顾
		</template>
		<template #text>
			<div class="review-options">
				<fieldset>
					<legend>项目范围</legend>
					<label class="choice"><input
						type="checkbox"
						:checked="allSelected"
						:indeterminate="selected.length > 0 && !allSelected"
						@change="selectAll(($event.target as HTMLInputElement).checked)"
					>全选（{{ selected.length }}/{{ projects.length }}）</label>
					<div class="review-projects">
						<label
							v-for="project in projects"
							:key="project.id"
							class="choice"
						>
							<input
								v-model="selected"
								type="checkbox"
								:value="project.id"
							>
							<span>{{ project.title }}</span>
						</label>
						<p v-if="!projects.length">
							暂无可选项目。
						</p>
					</div>
				</fieldset>
				<fieldset>
					<legend>时间范围</legend>
					<div class="range-mode">
						<label class="choice"><input
							v-model="range.mode"
							type="radio"
							:name="id"
							value="days"
						>过去多少天</label>
						<label class="choice"><input
							v-model="range.mode"
							type="radio"
							:name="id"
							value="dates"
						>指定日期范围</label>
					</div>
					<label
						v-if="range.mode === 'days'"
						class="range-field"
					>
						过去天数（含今天）
						<input
							v-model.number="range.days"
							class="input"
							type="number"
							min="1"
							max="36500"
							step="1"
						>
					</label>
					<div
						v-else
						class="review-dates"
					>
						<label class="range-field">开始日期<input
							v-model="range.from"
							class="input"
							type="date"
						></label>
						<label class="range-field">结束日期<input
							v-model="range.to"
							class="input"
							type="date"
						></label>
					</div>
				</fieldset>
				<p class="help">
					汇总期间保存的每日进展、新增遗留事项和备注修改。包含开始、结束当天；在新标签页查看并导出 Markdown。
				</p>
				<p
					v-if="error"
					class="review-error"
					role="alert"
				>
					{{ error }}
				</p>
			</div>
		</template>
	</Modal>
</template>

<script setup lang="ts">
import {computed, ref, useId, watch} from 'vue'
import {useRouter} from 'vue-router'
import {useProjectStore} from '@/stores/projects'
import XButton from '@/components/input/Button.vue'
import Modal from '@/components/misc/Modal.vue'
import {reviewDefaults, reviewRange} from '@/helpers/periodReview'
const store = useProjectStore(), router = useRouter(), id = useId()
const open = ref(false), error = ref(''), selected = ref<number[]>([])
const range = ref(reviewDefaults())
watch([selected, range], () => {error.value = ''}, {deep: true})
const projects = computed(() => store.projectsArray.filter(project => project.id > 0))
const allSelected = computed(() => projects.value.length > 0 && selected.value.length === projects.value.length)
function selectAll(checked: boolean) {selected.value = checked ? projects.value.map(project => project.id) : []}
function show() {selectAll(true); range.value = reviewDefaults(); error.value = ''; open.value = true}
function view() {
	error.value = ''
	try {
		if (!selected.value.length) throw new Error('请至少选择一个项目。')
		const dates = reviewRange(range.value)
		const href = router.resolve({name: 'tasktrace.review', query: {...dates, projects: selected.value.join(',')}}).href
		const tab = window.open(href, '_blank')
		if (!tab) throw new Error('浏览器阻止了新标签页，请允许弹出窗口后重试。')
		tab.opener = null
		open.value = false
	} catch (failure) {error.value = failure instanceof Error ? failure.message : '无法打开回顾，请重试。'}
}
</script>

<style scoped lang="scss">
.review-options {
 text-align: start;
 min-inline-size: 0;
 fieldset {
	border: 0;
	padding: 0;
	margin: 0 0 1.5rem;
}
 legend {
	font-weight: 600;
	margin-block-end: .75rem;
}
 .choice {
	display: flex;
	align-items: center;
	gap: .5rem;
	min-block-size: 2rem;
}
 input[type="checkbox"], input[type="radio"] {
	flex: 0 0 auto;
}
 .review-projects {
	max-block-size: 13rem;
	overflow: auto;
	margin-block-start: .5rem;
	padding: .5rem .75rem;
	border: 1px solid var(--grey-300);
	border-radius: .375rem;
}
 .review-projects span {
	overflow-wrap: anywhere;
}
 .range-mode {
	display: flex;
	flex-wrap: wrap;
	gap: .5rem 1.5rem;
	margin-block-end: .75rem;
}
 .range-field {
	display: flex;
	flex-direction: column;
	gap: .4rem;
}
 .review-dates {
	display: grid;
	grid-template-columns: repeat(2, minmax(0, 1fr));
	gap: .75rem;
}
 .input {
	inline-size: 100%;
	min-inline-size: 0;
}
 .review-error {
	color: var(--danger-text);
}
}
</style>
