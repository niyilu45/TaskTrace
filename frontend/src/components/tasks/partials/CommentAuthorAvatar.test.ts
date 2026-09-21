import {describe, expect, it, vi} from 'vitest'
import {mount} from '@vue/test-utils'

import CommentAuthorAvatar from './CommentAuthorAvatar.vue'

vi.mock('@/components/misc/UserAvatar.vue', () => ({
	default: {
		props: ['user', 'size'],
		template: '<span class="stored-user-avatar">{{ user && user.username }}</span>',
	},
}))

describe('CommentAuthorAvatar', () => {
	it('uses the shared profile avatar for a remote team author', () => {
		const wrapper = mount(CommentAuthorAvatar, {
			props: {
				user: {username: 'local-user'},
				author: 'DOMAIN\\remote-user',
				avatar: 'data:image/png;base64,remote',
				teamAuthored: true,
				size: 48,
			},
		})

		expect(wrapper.find('img').attributes('src')).toBe('data:image/png;base64,remote')
		expect(wrapper.find('.stored-user-avatar').exists()).toBe(false)
	})

	it('shows remote author initials instead of the stored local user avatar while a profile is unavailable', () => {
		const wrapper = mount(CommentAuthorAvatar, {
			props: {
				user: {username: 'local-user'},
				author: 'remote-user',
				teamAuthored: true,
				size: 20,
			},
		})

		expect(wrapper.find('.comment-author-avatar--fallback').text()).toBe('RE')
		expect(wrapper.find('.stored-user-avatar').exists()).toBe(false)
	})

	it('keeps the regular user avatar for comments authored by the stored user', () => {
		const wrapper = mount(CommentAuthorAvatar, {
			props: {
				user: {username: 'Local-User'},
				author: 'local-user',
				teamAuthored: true,
			},
		})

		expect(wrapper.find('.stored-user-avatar').text()).toBe('Local-User')
		expect(wrapper.find('img').exists()).toBe(false)
	})
})
