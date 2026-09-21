<template>
	<img
		v-if="showTeamAvatar"
		:src="avatar"
		alt=""
		:width="size"
		:height="size"
		class="comment-author-avatar"
	>
	<span
		v-else-if="showTeamFallback"
		:style="{'--comment-author-avatar-size': `${size}px`}"
		class="comment-author-avatar comment-author-avatar--fallback"
		aria-hidden="true"
	>
		{{ initials }}
	</span>
	<UserAvatar
		v-else
		:user="user"
		:size="size"
		alt=""
		class="comment-author-avatar"
	/>
</template>

<script setup lang="ts">
import {computed} from 'vue'

import UserAvatar from '@/components/misc/UserAvatar.vue'
import type {IUser} from '@/modelTypes/IUser'

const props = withDefaults(defineProps<{
	user?: Pick<IUser, 'username'> | null,
	author: string,
	avatar?: string,
	size?: number,
	teamAuthored?: boolean,
}>(), {
	user: undefined,
	avatar: '',
	size: 48,
	teamAuthored: false,
})

function memberKey(value = '') {
	return (value.trim().split('\\').pop()?.split('@')[0] || '').toLocaleLowerCase()
}

const isRemoteTeamAuthor = computed(() => props.teamAuthored && memberKey(props.author) !== memberKey(props.user?.username))
const showTeamAvatar = computed(() => isRemoteTeamAuthor.value && Boolean(props.avatar))
const showTeamFallback = computed(() => isRemoteTeamAuthor.value && !props.avatar)
const initials = computed(() => props.author.trim().slice(0, 2).toLocaleUpperCase() || '?')
</script>

<style lang="scss" scoped>
.comment-author-avatar {
	border-radius: 100%;
	flex: none;
	object-fit: cover;
}

.comment-author-avatar--fallback {
	display: inline-flex;
	align-items: center;
	justify-content: center;
	inline-size: var(--comment-author-avatar-size);
	block-size: var(--comment-author-avatar-size);
	background: var(--grey-200);
	color: var(--grey-700);
	font-size: calc(var(--comment-author-avatar-size) * .35);
	font-weight: 700;
	line-height: 1;
}
</style>
