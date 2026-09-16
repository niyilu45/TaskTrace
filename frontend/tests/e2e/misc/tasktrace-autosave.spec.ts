import {test, expect} from '@playwright/test'
import {readFileSync} from 'node:fs'
import path from 'node:path'

test('description saves changed content at configured intervals', async ({page, request}) => {
 const root = process.env.TASKTRACE_LOCAL_TEST_DIR
 test.skip(!root, 'Requires isolated portable server')
 const session = JSON.parse(readFileSync(path.join(root!, 'data/local-session.json'), 'utf8').replace(/^\uFEFF/, ''))
 const base = readFileSync(path.join(root!, 'data/local-config.yml'), 'utf8').match(/publicurl: "(http:\/\/127\.0\.0\.1:\d+)\/"/)![1]
 const headers = {Authorization: 'Bearer ' + session.token}
 const projects = await (await request.get(base + '/api/v2/projects', {headers})).json()
 const task = await (await request.post(base + '/api/v2/projects/' + projects.items[0].id + '/tasks', {headers, data: {title: 'Description autosave verification'}})).json()
 await page.addInitScript(() => localStorage.setItem('tasktrace-autosave', JSON.stringify({enabled: true, seconds: 5})))
 await page.goto(base + '/tasks/' + task.id + '#tasktrace-local=' + encodeURIComponent(JSON.stringify(session)))
 const editor = page.locator('.tiptap__task-description [contenteditable=true]').first()
 await expect(editor).toBeVisible()
 let writes = 0
 page.on('request', req => { if (req.url().endsWith('/tasks/' + task.id) && ['POST', 'PUT', 'PATCH'].includes(req.method())) writes++ })
 await editor.fill('描述按设置间隔自动保存')
 await expect.poll(async () => (await (await request.get(base + '/api/v2/tasks/' + task.id, {headers})).json()).description, {timeout: 12000}).toContain('描述按设置间隔自动保存')
 const afterSave = writes
 expect(afterSave).toBe(1)
 await page.waitForTimeout(5500)
 expect(writes).toBe(afterSave)
})
