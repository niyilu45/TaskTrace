<script setup lang="ts">
import {computed} from 'vue'
import type {TaskTraceTeamBindingStatus} from '@/client/generated'
import {formatDisplayDate} from '@/helpers/time/formatDate'
import TeamMemberIdentity from './TeamMemberIdentity.vue'

const props = defineProps<{binding: TaskTraceTeamBindingStatus, relatedBindings?: TaskTraceTeamBindingStatus[]}>()
const overlappingBindings = computed(() => {
	const taskIds = new Set([props.binding.root_task_id, ...(props.binding.task_ids ?? [])].filter(id => id != null))
	return (props.relatedBindings ?? []).filter(other => other.share_id !== props.binding.share_id &&
        [other.root_task_id, ...(other.task_ids ?? [])].some(id => id != null && taskIds.has(id)))
})
</script>

<template>
	<div class="team-sync-details">
		<p
			v-if="binding.last_sync"
			class="help"
		>
			最近成功同步：{{ formatDisplayDate(binding.last_sync) }}
		</p>
		<p
			v-if="binding.last_error"
			class="notification is-warning is-light"
			role="alert"
		>
			同步未完成：{{ binding.last_error }}。本地保存的内容仍保留；能打开权限页不代表同步已成功。
		</p>
		<details>
			<summary>协作连接信息</summary>
			<p>此任务在本机仍保留协作关系。</p>
			<dl>
				<dt>共享范围</dt>
				<dd>{{ binding.root_task_title || '共享任务' }}及其子任务</dd>
				<dt>所属人</dt>
				<dd><TeamMemberIdentity :username="binding.owner || ''" /></dd>
				<dt>协作标识</dt>
				<dd>{{ binding.share_id }}</dd>
				<dt>实际同步目录</dt>
				<dd>{{ binding.repository || '当前版本未提供目录信息，请更新程序后查看。' }}</dd>
			</dl>
			<p class="help">
				核对同一共享范围时，双方的协作标识应相同；同步目录必须指向同一份 teamData 数据。所属人的本地路径与成员的网络共享路径可以不同。
			</p>
			<div
				v-if="overlappingBindings.length"
				class="team-sync-details__overlap"
			>
				<p>检测到重叠的协作范围。另行分享父任务会生成独立链接，旧子任务链接不会自动扩展到父任务。</p>
				<p>以下共享范围包含相同的本地任务，请与成员核对正在使用的协作标识：</p>
				<dl
					v-for="other in overlappingBindings"
					:key="other.share_id"
				>
					<dt>共享范围</dt><dd>{{ other.root_task_title || '共享任务' }}及其子任务</dd>
					<dt>协作标识</dt><dd>{{ other.share_id }}</dd>
					<dt>实际同步目录</dt><dd>{{ other.repository || '当前版本未提供' }}</dd>
					<template v-if="other.last_error">
						<dt>同步错误</dt><dd>{{ other.last_error }}</dd>
					</template>
				</dl>
			</div>
		</details>
	</div>
</template>

<style scoped lang="scss">
.team-sync-details {
	flex-basis: 100%;
	min-inline-size: 0;
	color: var(--text);
	overflow-wrap: anywhere;

	&__overlap {
		margin-block-start: .75rem;
		padding-block-start: .75rem;
		border-block-start: 1px solid var(--grey-200);
	}

	summary {
		cursor: pointer;
		font-weight: 600;
		color: var(--primary);
	}

	dl {
		display: grid;
		grid-template-columns: max-content minmax(0, 1fr);
		gap: .25rem 1rem;
		margin-block: .5rem;
	}

	dt { font-weight: 600; }
	dd { margin: 0; }
}
</style>
