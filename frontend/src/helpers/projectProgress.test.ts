import {describe, it, expect} from 'vitest'
import {groupProgressTasks, visibleProgressRows, type ProgressTask} from './projectProgress'
const task = (id: number, parents: number[] = []): ProgressTask => ({id, title: String(id), related_tasks: {parenttask: parents.map(id => ({id}))}})
describe('project progress grouping', () => {
 it('keeps descendants together, including completed children', () => {
  const groups = groupProgressTasks([task(1), {...task(2, [1]), done: true}, task(3, [2]), task(4)])
  expect(groups.map(group => group.rows.map(row => [row.task.id, row.depth]))).toEqual([[[1, 0], [2, 1], [3, 2]], [[4, 0]]])
 })
 it('retains every task once with cycles, multiple parents and external parents', () => {
  const groups = groupProgressTasks([task(1, [2]), task(2, [1]), task(3, [99]), task(4, [1, 3])])
  const ids = groups.flatMap(group => group.rows.map(row => row.task.id))
  expect(ids.sort()).toEqual([1, 2, 3, 4])
 })
})

describe('overview hierarchy visibility', () => {
 const rows = groupProgressTasks([task(1), {...task(2, [1]), done: true}, task(3, [2]), task(4, [1]), task(5, [4])])[0].rows
 it('collapses only descendants, leaving sibling branches visible', () => {
  expect(visibleProgressRows(rows, 'all', '', new Set([2])).map(row => row.task.id)).toEqual([1, 2, 4, 5])
  expect(visibleProgressRows(rows, 'all', '', new Set([1])).map(row => row.task.id)).toEqual([1])
 })
 it('retains completed ancestors of pending children', () => {
  expect(visibleProgressRows(rows, 'pending', '', new Set()).map(row => row.task.id)).toEqual([1, 2, 3, 4, 5])
  expect(visibleProgressRows(rows, 'done', '', new Set()).map(row => row.task.id)).toEqual([1, 2])
 })
 it('reveals the whole ancestor chain for search without losing saved collapse', () => {
  const collapsed = new Set([1, 2])
  expect(visibleProgressRows(rows, 'all', '3', collapsed).map(row => [row.task.id, row.depth])).toEqual([[1, 0], [2, 1], [3, 2]])
  expect(visibleProgressRows(rows, 'all', '', collapsed).map(row => row.task.id)).toEqual([1])
 })
})
