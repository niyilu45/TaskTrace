import {test, expect} from '@playwright/test'
import {readFileSync} from 'node:fs'
import path from 'node:path'

test('create a child in the same project and open its own progress', async ({page, request}) => {
 const root = process.env.TASKTRACE_LOCAL_TEST_DIR
 test.skip(!root, 'Requires isolated portable instance')
 const session = JSON.parse(readFileSync(path.join(root!, 'data/local-session.json'), 'utf8').replace(/^\uFEFF/, ''))
 const base = readFileSync(path.join(root!, 'data/local-config.yml'), 'utf8').match(/publicurl: "(http:\/\/127\.0\.0\.1:\d+)\/"/)![1]
 const headers = {Authorization: 'Bearer ' + session.token}
 const projects = await (await request.get(base + '/api/v2/projects', {headers})).json()
 const parent = await (await request.post(base + '/api/v2/projects/' + projects.items[0].id + '/tasks', {headers, data: {title: '父事项验收'}})).json()
 await page.goto(base + '/tasks/' + parent.id + '#tasktrace-local=' + encodeURIComponent(JSON.stringify(session)))
 await page.getByRole('textbox', {name: '子任务名称'}).fill('拆分子任务验收')
 await page.getByRole('button', {name: '添加子任务', exact: true}).click()
 await expect(page.getByRole('status').filter({hasText: '子任务已添加'})).toBeVisible()
 const saved = await (await request.get(base + '/api/v2/tasks/' + parent.id, {headers})).json()
 expect(saved.related_tasks.subtask).toHaveLength(1)
 const child = saved.related_tasks.subtask[0]
 expect(child.project_id).toBe(parent.project_id)
 await page.getByRole('button', {name: '修改子任务名称：拆分子任务验收', exact: true}).click()
 const nameInput = page.getByRole('textbox', {name: '修改子任务名称', exact: true})
 await nameInput.fill('   ')
 await expect(page.getByRole('button', {name: '保存名称', exact: true})).toBeDisabled()
 await nameInput.fill('不应保存')
 await page.getByRole('button', {name: '取消', exact: true}).click()
 await page.getByRole('button', {name: '修改子任务名称：拆分子任务验收', exact: true}).click()
 await nameInput.fill('修改后的子任务')
 await nameInput.press('Enter')
 await expect(page.getByRole('status').filter({hasText: '子任务名称已保存'})).toBeVisible()
 const renamed = await (await request.get(base + '/api/v2/tasks/' + child.id, {headers})).json()
 expect(renamed.title).toBe('修改后的子任务')
 expect(renamed.related_tasks.parenttask.map((task: {id: number}) => task.id)).toContain(parent.id)
 await page.reload()
 await page.locator('.task-relations .task').getByRole('link').filter({hasText: '修改后的子任务'}).click()
 await expect(page).toHaveURL(new RegExp('/tasks/' + child.id + '$'))
 await expect(page.getByRole('dialog').getByRole('heading', {name: '记录每日进展'})).toBeVisible()
 await request.patch(base + '/api/v2/tasks/' + child.id, {headers: {...headers, 'Content-Type': 'application/merge-patch+json'}, data: {done: true}})
 const unchanged = await (await request.get(base + '/api/v2/tasks/' + parent.id, {headers})).json()
 expect(unchanged.done).toBe(false)
})
