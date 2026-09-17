import {beforeEach, describe, expect, it} from 'vitest'
import {mount} from '@vue/test-utils'
import {createPinia, setActivePinia} from 'pinia'
import {createI18n} from 'vue-i18n'
import CalendarMonth from './CalendarMonth.vue'
import en from '@/i18n/lang/en.json'

const i18n = createI18n({legacy: false, locale: 'en', messages: {en}})

describe('CalendarMonth progress markers', () => {
	beforeEach(() => setActivePinia(createPinia()))

	it('marks only dates with saved progress and describes the marker', () => {
		const wrapper = mount(CalendarMonth, {
			props: {
				selected: new Date(2026, 8, 18),
				markedDates: ['2026-09-17', '2026-09-18'],
			},
			global: {plugins: [i18n], stubs: {BaseButton: true, Icon: true}},
		})
		const marked = wrapper.findAll('.calendar-month__day.has-marker')
		expect(marked).toHaveLength(2)
		expect(wrapper.get('[data-date="2026-09-17"]').attributes('aria-label')).toContain('有进展')
		expect(wrapper.get('[data-date="2026-09-16"]').classes()).not.toContain('has-marker')
	})
})
