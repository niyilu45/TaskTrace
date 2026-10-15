<template>
	<XButton
		variant="secondary"
		icon="link"
		@click="openImport"
	>
		{{ buttonLabel }}
	</XButton>
	<Modal
		:enabled="showImport"
		cancel-label="取消"
		:submit-label="submitting ? '正在导入…' : '导入任务'"
		:aria-busy="submitting"
		@close="closeImport"
		@submit="importTask"
	>
		<template #header>
			导入团队任务链接
		</template>
		<template #text>
			<fieldset
				class="import-fields"
				:disabled="submitting"
				@input="importError = ''"
			>
				<div class="field">
					<label
						class="label"
						:for="id + '-link'"
					>任务链接</label>
					<textarea
						:id="id + '-link'"
						v-model="link"
						class="textarea"
						rows="3"
						placeholder="粘贴以 tasktrace-team:// 开头的链接"
					/>
					<p class="help">
						导入共享任务及其子任务。已存在的协作任务会合并本地修改并更新同步地址，保留原来的任务位置。
					</p>
				</div>
				<div class="field">
					<label
						class="label"
						:for="id + '-repository'"
					>共享目录地址（可选）</label>
					<input
						:id="id + '-repository'"
						v-model="repositoryOverride"
						class="input"
						placeholder="\\10.143.58.8\teamData"
					>
					<p class="help">
						链接中的电脑名无法访问时，填写完整共享地址；若包含 imports 子目录，需一并保留。
					</p>
				</div>
				<div class="field">
					<label
						class="label"
						:for="id + '-project'"
					>保存到项目</label>
					<div class="select is-fullwidth">
						<select
							:id="id + '-project'"
							v-model="destination"
						>
							<option
								value=""
								disabled
							>
								请选择项目
							</option>
							<option
								v-for="project in writableProjects"
								:key="project.id"
								:value="String(project.id)"
							>
								{{ project.title }}
							</option>
							<option value="new">
								＋ 新建项目…
							</option>
						</select>
					</div>
				</div>
				<div
					v-if="destination === 'new'"
					class="field"
				>
					<label
						class="label"
						:for="id + '-name'"
					>新项目名称</label>
					<input
						:id="id + '-name'"
						v-model="newProjectName"
						class="input"
						maxlength="250"
						placeholder="输入用于保存共享任务的项目名称"
						@keydown.enter.prevent="importTask"
					>
					<p class="help">
						点击“导入任务”后创建项目，再导入共享任务。
					</p>
				</div>
			</fieldset>
			<p
				v-if="importError"
				class="notification is-danger is-light has-text-danger"
				role="alert"
			>
				{{ importError }}
			</p>
			<div
				v-if="teamStore.status.repository && !teamStore.status.repository.shared"
				class="notification is-warning is-light"
			>
				本机 teamData 尚未检测到 Windows 共享。接收别人链接不受影响；若要分享自己的任务，请先在文件夹属性中共享 teamData。
			</div>
		</template>
	</Modal>
</template>

<script setup lang="ts">
import {computed, ref, useId} from 'vue'

import XButton from '@/components/input/Button.vue'
import Modal from '@/components/misc/Modal.vue'
import {projectsCreate} from '@/client/generated'
import {PERMISSIONS} from '@/constants/permissions'
import {objectToCamelCase} from '@/helpers/case'
import {getErrorText, success} from '@/message'
import ProjectModel from '@/models/project'
import {useProjectStore} from '@/stores/projects'
import {useTasktraceTeamStore} from '@/stores/tasktraceTeam'

const props = withDefaults(defineProps<{projectId?: number, buttonLabel?: string}>(), {
	projectId: 0,
	buttonLabel: '导入共享任务链接',
})
const emit = defineEmits<{imported: []}>()
const id = useId()
const teamStore = useTasktraceTeamStore()
const projectStore = useProjectStore()
const writableProjects = computed(() => projectStore.projectsArray.filter(project =>
	project.id > 0 && !project.isArchived && (project.maxPermission ?? 0) >= PERMISSIONS.READ_WRITE,
))
const showImport = ref(false)
const submitting = ref(false)
const importError = ref('')
const link = ref('')
const repositoryOverride = ref('')
const destination = ref('')
const newProjectName = ref('')

function openImport() {
	destination.value = writableProjects.value.some(project => project.id === props.projectId)
		? String(props.projectId)
		: writableProjects.value.length ? '' : 'new'
	showImport.value = true
}

function closeImport() {
	if (submitting.value) return
	showImport.value = false
	link.value = ''
	repositoryOverride.value = ''
	newProjectName.value = ''
	importError.value = ''
}

async function importTask() {
	if (submitting.value) return
	importError.value = ''
	if (!link.value.trim()) {
		importError.value = '请先粘贴共享任务链接。'
		return
	}
	if (!destination.value) {
		importError.value = '请选择保存到的项目，或选择新建项目。'
		return
	}
	if (destination.value === 'new' && !newProjectName.value.trim()) {
		importError.value = '请输入新项目名称。'
		return
	}
	if (destination.value !== 'new' && !writableProjects.value.some(project => String(project.id) === destination.value)) {
		importError.value = '该项目不可写入，请重新选择项目。'
		return
	}
	submitting.value = true
	try {
		if (destination.value === 'new') {
			const {data} = await projectsCreate({body: {title: newProjectName.value.trim()}})
			const project = new ProjectModel({...objectToCamelCase(data), maxPermission: PERMISSIONS.ADMIN})
			projectStore.setProject(project)
			// Keep the created destination on failure so retrying never creates another project.
			destination.value = String(project.id)
			newProjectName.value = ''
		}
		await teamStore.importLink(link.value.trim(), Number(destination.value), repositoryOverride.value)
		success({message: '协作连接已同步。新任务已导入；已有任务保留原位置。'})
		submitting.value = false
		closeImport()
		emit('imported')
	} catch (cause) {
		importError.value = getErrorText(cause) || '导入失败，请检查任务链接和共享目录后重试。'
	} finally {
		submitting.value = false
	}
}
</script>

<style scoped lang="scss">
.import-fields {
	border: 0;
	padding: 0;
	margin: 0;
	min-inline-size: 0;
	text-align: start;
}
</style>
