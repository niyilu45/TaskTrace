import {test, expect} from '@playwright/test'
import {readFileSync} from 'node:fs'
import path from 'node:path'

test('overview shows completed tasks when selected and remembers the selection', async ({page, request}) => {
 const root = process.env.TASKTRACE_LOCAL_TEST_DIR
 test.skip(!root, 'Requires isolated portable server')
 const session = JSON.parse(readFileSync(path.join(root!, 'data/local-session.json'), 'utf8').replace(/^\uFEFF/, ''))
 const base = readFileSync(path.join(root!, 'data/local-config.yml'), 'utf8').match(/publicurl: "(http:\/\/127\.0\.0\.1:\d+)\/"/)![1]
 const headers = {Authorization: 'Bearer ' + session.token}
 const projects = await (await request.get(base + '/api/v2/projects', {headers})).json()
 const name = '完成筛选验收 ' + Date.now()
 const pendingName = '未完成筛选验收 ' + Date.now()
 const doneTask = await (await request.post(base + '/api/v2/projects/' + projects.items[0].id + '/tasks', {headers, data: {title: name, done: true}})).json()
 await request.post(base + '/api/v2/projects/' + projects.items[0].id + '/tasks', {headers, data: {title: pendingName}})
 await page.goto(base + '/#tasktrace-local=' + encodeURIComponent(JSON.stringify(session)))
 await expect(page.getByText(pendingName, {exact: true})).toBeVisible()
 await expect(page.getByText(name, {exact: true})).toHaveCount(0)
 await page.getByLabel('任务显示范围').selectOption('all')
 await expect(page.getByText(name, {exact: true})).toBeVisible()
 await expect(page.getByText(pendingName, {exact: true})).toBeVisible()
 await page.getByLabel('任务显示范围').selectOption('done')
 await expect(page.getByText(pendingName, {exact: true})).toHaveCount(0)
 await expect(page.getByText(name, {exact: true})).toBeVisible()
 await page.reload()
 await expect(page.getByLabel('任务显示范围')).toHaveValue('done')
 await expect(page.getByText(name, {exact: true})).toBeVisible()
 await page.getByText(name, {exact: true}).click()
 await expect(page).toHaveURL(new RegExp('/tasks/' + doneTask.id + '$'))
})
