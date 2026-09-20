<template>
	<div
		v-if="members.length > 1"
		class="task-collaboration"
	>
		<button
			type="button"
			class="task-collaboration__trigger"
			:aria-expanded="expanded"
			:aria-label="`${expanded ? '收起' : '展开'}协作成员，共 ${members.length} 人`"
			@click.stop="expanded = !expanded"
		>
			<Icon icon="users" />
			{{ members.length }} 人协作
			<Icon
				icon="chevron-down"
				class="task-collaboration__chevron"
				:class="{'is-expanded': expanded}"
			/>
		</button>
		<ul
			v-if="expanded"
			class="task-collaboration__members"
		>
			<li
				v-for="member in members"
				:key="member.toLowerCase()"
			>
				<img
					v-if="avatarFor(member)"
					:src="avatarFor(member)"
					alt=""
				>
				<span
					v-else
					class="task-collaboration__avatar"
				>{{ initials(member) }}</span>
				<span>{{ member }}</span>
			</li>
		</ul>
	</div>
</template>

<script setup lang="ts">
import {computed, onMounted, ref} from 'vue'

import Icon from '@/components/misc/Icon'
import {isLocalBuild} from '@/helpers/tasktraceLocal'
import {collaborationMembers} from '@/helpers/tasktraceTeam'
import {useTasktraceTeamStore} from '@/stores/tasktraceTeam'

const props = defineProps<{taskId: number}>()
const teamStore = useTasktraceTeamStore()
const expanded = ref(false)
const binding = computed(() => teamStore.bindingForTask(props.taskId))
const members = computed(() => collaborationMembers(binding.value, teamStore.status.username))

function avatarFor(username: string) {
	return teamStore.status.profiles?.find(profile => profile.username?.toLowerCase() === username.toLowerCase())?.avatar || ''
}

function initials(username: string) {
	return username.trim().slice(0, 2).toUpperCase() || '?'
}

onMounted(() => {
	if (isLocalBuild) void teamStore.refresh().catch(() => undefined)
})
</script>

<style scoped lang="scss">
.task-collaboration {
	margin-block-start: .25rem;
	font-weight: 400;
}

.task-collaboration__trigger {
	display: inline-flex;
	align-items: center;
	gap: .3rem;
	min-block-size: 1.75rem;
	padding: .1rem .35rem;
	border: 0;
	border-radius: .25rem;
	background: transparent;
	color: var(--grey-600);
	font: inherit;
	font-size: .75rem;
	cursor: pointer;
	&:hover {
		background: var(--grey-100);
		color: var(--text);
	}
	&:focus-visible {
		outline: 2px solid var(--primary);
		outline-offset: 2px;
	}
}

.task-collaboration__chevron {
	font-size: .65rem;
	transition: transform $transition;
	&.is-expanded { transform: rotate(180deg); }
}

.task-collaboration__members {
	display: flex;
	flex-wrap: wrap;
	gap: .35rem .65rem;
	margin: .25rem 0 0;
	padding: 0;
	list-style: none;
	li {
		display: inline-flex;
		align-items: center;
		gap: .3rem;
		color: var(--grey-700);
		font-size: .75rem;
	}
	img, .task-collaboration__avatar {
		inline-size: 1.35rem;
		block-size: 1.35rem;
		border-radius: 50%;
	}
	img { object-fit: cover; }
}

.task-collaboration__avatar {
	display: inline-grid;
	place-items: center;
	background: var(--primary);
	color: var(--white);
	font-size: .55rem;
	font-weight: 700;
}
</style>
