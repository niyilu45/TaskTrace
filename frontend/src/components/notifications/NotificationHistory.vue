<template>
	<section
		class="notification-history"
		aria-label="历史通知"
	>
		<p class="history-help">
			清除的通知会收纳在这里，不再计入未读数量。保留数量可在设置中修改。
		</p>
		<p
			v-if="loading"
			role="status"
		>
			正在读取历史通知…
		</p>
		<div
			v-else-if="loadError"
			role="alert"
		>
			<p>{{ loadError }}</p>
			<XButton
				variant="secondary"
				@click="load"
			>
				重试
			</XButton>
		</div>
		<p
			v-else-if="!entries.length"
			class="history-help"
		>
			暂无历史通知。清除通知后可在此查看。
		</p>
		<ol
			v-else
			class="history-list"
		>
			<li
				v-for="entry in entries"
				:key="entry.id"
			>
				<button
					v-if="entry.team"
					type="button"
					class="history-link"
					@click="$emit('openTeam', entry.team)"
				>
					<TeamMemberIdentity :username="entry.team.actor || '协作成员'" />
					更新了“{{ entry.team.task_title || '团队任务' }}”
				</button>
				<button
					v-else-if="entry.model"
					type="button"
					class="history-link"
					@click="$emit('openNotification', entry.model)"
				>
					{{ entry.model.notification.doer ? getDisplayName(entry.model.notification.doer) : '' }}
					{{ entry.model.toText(userInfo) }}
					<span v-if="entry.model.name === 'tasktrace.export.completed'"> · 打开所在文件夹</span>
				</button>
				<time :datetime="entry.archived_at">收纳于 {{ formatDate(entry.archived_at) }}</time>
			</li>
		</ol>
	</section>
</template>

<script setup lang="ts">
import {computed, onMounted, ref} from 'vue'
import {tasktraceNotificationHistoryList, type TaskTraceNotificationHistoryEntry, type TaskTraceTeamNotification} from '@/client/generated'
import NotificationModel from '@/models/notification'
import type {INotification} from '@/modelTypes/INotification'
import UserModel, {getDisplayName} from '@/models/user'
import {useAuthStore} from '@/stores/auth'
import {getErrorText} from '@/message'
import XButton from '@/components/input/Button.vue'
import TeamMemberIdentity from '@/components/tasks/partials/TeamMemberIdentity.vue'

defineEmits<{openTeam: [notice: TaskTraceTeamNotification], openNotification: [notice: INotification]}>()
const authStore = useAuthStore()
const userInfo = computed(() => authStore.info ? new UserModel({id: authStore.info.id}) : null)
const rows = ref<TaskTraceNotificationHistoryEntry[]>([])
const entries = computed(() => rows.value.map(entry => ({...entry, model: entry.notification ? new NotificationModel(entry.notification as Partial<INotification>) : null})))
const loading = ref(false)
const loadError = ref('')
const formatDate = (value?: string) => value ? new Date(value).toLocaleString() : '时间未知'
async function load() {
	if (loading.value) return
	loading.value = true
	loadError.value = ''
	try { rows.value = (await tasktraceNotificationHistoryList()).data || [] }
	catch (cause) { loadError.value = `无法读取历史通知：${getErrorText(cause)}` }
	finally { loading.value = false }
}
onMounted(load)
</script>

<style scoped lang="scss">
.notification-history {
 color: var(--text);
}
.history-help {
 margin: .75rem;
 color: var(--text);
 line-height: 1.6;
}
.history-list {
 list-style: none;
 margin: 0;
 padding: 0;
}
.history-list li {
 padding: .75rem;
 border-block-end: 1px solid var(--grey-200);
}
.history-list time {
 display: block;
 margin-block-start: .35rem;
 color: var(--grey-700);
 font-size: .85rem;
}
.history-link {
 border: 0;
 padding: 0;
 background: transparent;
 color: var(--link);
 font: inherit;
 text-align: start;
 overflow-wrap: anywhere;
 cursor: pointer;
}
.history-link:hover,
.history-link:focus-visible {
 text-decoration: underline;
}
</style>
