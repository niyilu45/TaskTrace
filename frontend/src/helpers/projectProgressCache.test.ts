import {beforeEach, describe, expect, it} from 'vitest'
import {clearProjectProgressCache, readProjectProgressCache, writeProjectProgressCache} from './projectProgressCache'
import type {ProgressTask} from './projectProgress'

function task(id: number): ProgressTask {
	return {id, title: `Task ${id}`} as ProgressTask
}

beforeEach(clearProjectProgressCache)

describe('project progress cache', () => {
	it('returns a previously loaded project immediately', () => {
		writeProjectProgressCache(1, [task(11)])
		expect(readProjectProgressCache(1).map(item => item.id)).toEqual([11])
		expect(readProjectProgressCache(2)).toEqual([])
	})

	it('keeps only the eight most recently used projects', () => {
		for (let projectId = 1; projectId <= 9; projectId++) writeProjectProgressCache(projectId, [task(projectId)])
		expect(readProjectProgressCache(1)).toEqual([])
		expect(readProjectProgressCache(9).map(item => item.id)).toEqual([9])
	})
})
