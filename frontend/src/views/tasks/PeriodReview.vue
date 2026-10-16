<template>
	<main class="period-review">
		<div class="preview-toolbar">
			<div>
				<h1>周期回顾</h1><p v-if="report">
					共 {{ report.task_count }} 个任务 · {{ report.change_count }} 条回顾内容
				</p>
			</div>
			<XButton
				v-if="report"
				variant="secondary"
				icon="download"
				:disabled="saving"
				@click="save"
			>
				导出 Markdown
			</XButton>
		</div>
		<p
			v-if="loading"
			role="status"
		>
			正在汇总所选项目的进展和遗留事项变化…
		</p>
		<div
			v-else-if="failure"
			role="alert"
		>
			<p>{{ failure }}</p><XButton
				variant="secondary"
				@click="load"
			>
				重试
			</XButton>
		</div>
		<template v-else-if="report">
			<p
				v-for="warning in report.warnings"
				:key="warning"
				class="notification is-warning"
			>
				{{ warning }}
			</p>
			<p
				v-if="saveMessage"
				role="status"
			>
				{{ saveMessage }}
			</p>
			<p class="review-scope">
				{{ scope }}
			</p>
			<p class="review-hint">
				遗留事项显示最新内容：周期内新增、完成或修改分别标注；无变化的未完成事项不加标志，周期外完成的事项不展示。拖动表头分隔线可调整列宽。
			</p>
			<div
				v-if="projects.length"
				class="review-layout"
			>
				<nav
					class="review-navigation"
					aria-label="周期回顾目录"
				>
					<details open>
						<summary>项目与任务</summary>
						<div class="review-directory">
							<div
								v-for="project in projects"
								:key="project.id"
								class="directory-project"
							>
								<a
									:href="'#review-project-' + project.id"
									:aria-current="active === 'review-project-' + project.id ? 'location' : undefined"
									@click.prevent="navigate('review-project-' + project.id)"
								>{{ project.title }}</a>
								<a
									v-for="task in project.tasks"
									:key="task.id"
									class="directory-task"
									:href="'#review-task-' + task.id"
									:aria-current="active === 'review-task-' + task.id ? 'location' : undefined"
									@click.prevent="navigate('review-task-' + task.id)"
								>{{ task.title }}</a>
							</div>
						</div>
					</details>
				</nav>
				<article
					ref="documentElement"
					class="review-document"
					aria-label="周期回顾展示模式"
				>
					<section
						v-for="project in projects"
						:id="'review-project-' + project.id"
						:key="project.id"
						class="review-project"
						tabindex="-1"
					>
						<header><h2>{{ project.title }}</h2><span>{{ project.tasks?.length || 0 }} 个任务</span></header>
						<ProjectProgressTable
							:labels="['任务名', '任务描述', '遗留事项', '进展']"
							:widths="widths"
							:label="project.title + '周期回顾'"
							@resize="resizeColumns"
						>
							<tr
								v-for="task in project.tasks"
								:id="'review-task-' + task.id"
								:key="task.id"
								class="review-task"
								:class="{'is-current': active === 'review-task-' + task.id}"
								tabindex="-1"
							>
								<th scope="row">
									{{ task.title }}
								</th>
								<td>
									<ReadonlyRichText
										v-if="task.description"
										:html="task.description"
										:deduplicate-images="false"
									/><span
										v-else
										class="empty"
									>暂无描述</span>
								</td>
								<td>
									<ReadonlyRichText
										v-if="task.outstanding"
										:html="task.outstanding"
										:deduplicate-images="false"
									/><span
										v-else
										class="empty"
									>无符合条件的遗留事项</span>
								</td>
								<td>
									<ReadonlyRichText
										v-if="task.progress"
										:html="task.progress"
										:deduplicate-images="false"
									/><span
										v-else
										class="empty"
									>期间无进展更新</span>
								</td>
							</tr>
						</ProjectProgressTable>
					</section>
				</article>
			</div>
			<p
				v-else
				role="status"
			>
				所选项目在此时间范围内没有可回顾的进展、未完成遗留事项或周期内完成的遗留事项。
			</p>
		</template>
	</main>
</template>

<script setup lang="ts">
import {ref, watch, onBeforeUnmount} from 'vue'
import {useRoute} from 'vue-router'
import {useResizeObserver, useMediaQuery} from '@vueuse/core'
import {marked} from 'marked'
import {tasktracePeriodReview, type TaskTracePeriodReview, type TaskTraceReviewProject} from '@/client/generated'
import ReadonlyRichText from '@/components/tasks/partials/ReadonlyRichText.vue'
import XButton from '@/components/input/Button.vue'
import {saveMarkdownDocument} from '@/helpers/taskMarkdown'
import ProjectProgressTable from '@/components/project/views/ProjectProgressTable.vue'
const route = useRoute()
const loading = ref(false), saving = ref(false), failure = ref(''), saveMessage = ref('')
const report = ref<TaskTracePeriodReview>()
const projects = ref<TaskTraceReviewProject[]>([])
const widths = ref([160, 200, 300, 360])
const documentElement = ref<HTMLElement>()
const desktop = useMediaQuery('(min-width: 901px)')
const availableWidth = ref(0)
let customWidths = false
function resizeColumns(value: number[]) {
	customWidths = true
	widths.value = value
}
useResizeObserver(documentElement, ([entry]) => { availableWidth.value = entry.contentRect.width })
watch([availableWidth, desktop], ([available, isDesktop]) => {
	if (customWidths || !available) return
	if (!isDesktop) { widths.value = [160, 200, 300, 360]; return }
	const extra = Math.max(0, Math.floor(available) - 560)
	widths.value = [144, 96 + Math.floor(extra * .2), 140 + Math.floor(extra * .35), 180 + extra - Math.floor(extra * .2) - Math.floor(extra * .35)]
})
const active = ref(''), scope = ref('')
function navigate(id: string) {
	const element = document.getElementById(id)
	if (!element) return
	active.value = id
	element.focus({preventScroll: true})
	element.scrollIntoView({block: 'start', inline: 'nearest'})
}
async function renderMarkdown(markdown?: string) {
	const rendered = new DOMParser().parseFromString(await marked.parse(markdown || '', {gfm: true}), 'text/html')
	for (const checkbox of rendered.querySelectorAll<HTMLInputElement>('input[type=checkbox]')) checkbox.replaceWith(document.createTextNode(checkbox.checked ? '☑ ' : '☐ '))
	return rendered.body.innerHTML
}
let controller: AbortController | undefined
async function load() {
	controller?.abort(); const request = controller = new AbortController()
	loading.value = true; failure.value = ''; report.value = undefined; projects.value = []; active.value = ''; saveMessage.value = ''
	try {
		const projectIds = String(route.query.projects || '').split(',').map(Number)
		const from = String(route.query.from || ''), until = String(route.query.until || '')
		if (projectIds.some(id => !Number.isSafeInteger(id) || id < 1) || !from || !until) throw new Error('范围无效')
		const {data} = await tasktracePeriodReview({throwOnError: true, query: {projects: projectIds, from, until}, querySerializer: {array: {explode: false, style: 'form'}}, signal: request.signal})
		const rendered = await Promise.all((data.projects || []).map(async project => ({
			...project,
			tasks: await Promise.all((project.tasks || []).map(async task => ({
				...task,
				description: await renderMarkdown(task.description),
				outstanding: await renderMarkdown(task.outstanding),
				progress: await renderMarkdown(task.progress),
			}))),
		})))
		if (controller !== request) return
		report.value = data; projects.value = rendered
		scope.value = (data.markdown || '').split('\n').find(line => line.startsWith('时间范围：')) || ''
	} catch {
		if (!request.signal.aborted) failure.value = '无法生成回顾。请检查连接和项目访问权限，或回到概览重新选择范围。'
	} finally {if (controller === request) loading.value = false}
}
async function save() {
	if (!report.value || saving.value) return
	saving.value = true; saveMessage.value = ''
	try {await saveMarkdownDocument(report.value.markdown || '', report.value.filename || '周期回顾.md'); saveMessage.value = 'Markdown 文件已保存。'}
	catch (error) {saveMessage.value = error instanceof Error && error.name === 'AbortError' ? '已取消导出。' : '导出失败，请重试。'}
	finally {saving.value = false}
}
watch(() => [route.query.projects, route.query.from, route.query.until], load, {immediate: true})
onBeforeUnmount(() => controller?.abort())
</script>

<style scoped lang="scss">
.period-review {
	max-inline-size: 100rem;
	margin-inline: auto;
	padding: 1.5rem;
}
.preview-toolbar {
	display: flex;
	align-items: center;
	justify-content: space-between;
	flex-wrap: wrap;
	gap: 1rem;
	margin-block-end: 1rem;
	h1 {
		margin: 0;
		font-size: 1.25rem;
	}
}
.review-scope { font-weight: 600; }
.review-hint {
	font-size: .875rem;
	color: var(--grey-700);
	margin-block-end: 1rem;
}
.review-layout {
	display: grid;
	grid-template-columns: 13rem minmax(0, 1fr);
	gap: 1.25rem;
	align-items: start;
}
.review-navigation {
	position: sticky;
	inset-block-start: 5rem;
	border: 1px solid var(--grey-200);
	border-radius: .375rem;
	padding: .75rem;
	background: var(--white);
	summary {
		font-weight: 600;
		cursor: pointer;
		padding-block-end: .5rem;
	}
	a {
		display: block;
		padding: .4rem .5rem;
		overflow-wrap: anywhere;
		color: var(--grey-900);
		text-decoration: none;
	}
	a:hover, a[aria-current] {
		background: var(--grey-100);
		color: var(--primary);
	}
	a:focus-visible {
		outline: 2px solid var(--primary);
		outline-offset: -2px;
	}
	.directory-task {
		padding-inline-start: 1.25rem;
		font-size: .875rem;
	}
}
.review-directory {
	max-block-size: 65vh;
	overflow-y: auto;
}
.directory-project + .directory-project { margin-block-start: .75rem; }
.review-document { min-inline-size: 0; }
.review-project {
	border: 1px solid var(--grey-200);
	border-radius: .375rem;
	margin-block-end: 1rem;
	background: var(--white);
	scroll-margin-block-start: 5rem;
	> header {
		display: flex;
		align-items: baseline;
		gap: .75rem;
		padding: .75rem;
		flex-wrap: wrap;
	}
	h2 {
		margin: 0;
		font-size: 1rem;
	}
	header span {
		font-size: .8125rem;
		color: var(--grey-700);
	}
}
.review-task {
	scroll-margin-block-start: 5rem;
	th, td {
		vertical-align: top;
		text-align: start;
		padding: .75rem;
		border-block-start: 1px solid var(--grey-200);
		overflow-wrap: anywhere;
	}
	th { font-weight: 600; }
	td { font-size: .875rem; }
	&.is-current { background: var(--grey-50); }
	&:focus-visible {
		outline: 2px solid var(--primary);
		outline-offset: -2px;
	}
	:deep(h4) {
		font-size: .875rem;
		font-weight: 600;
		margin-block: .75rem .5rem;
	}
	:deep(h4:first-child) { margin-block-start: 0; }
	:deep(p) { margin-block: 0 .5rem; }
	:deep(ul), :deep(ol) {
		padding-inline-start: 1.25rem;
		margin-block: .5rem;
	}
	:deep(ul) { list-style: disc; }
	:deep(ol) { list-style: decimal; }
}
.empty { color: var(--grey-600); }
@media (width <= 900px) {
	.period-review { padding: .75rem; }
	.review-layout { grid-template-columns: minmax(0, 1fr); }
	.review-navigation { position: static; }
	.review-directory { max-block-size: 12rem; }
}
</style>
