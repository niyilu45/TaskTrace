import {test, expect} from '../../support/fixtures'

test('export notifications show failure causes and locate only successfully saved files', async ({authenticatedPage: page}, testInfo) => {
 const savedPath = 'D:/导出目录/很长的项目名称与交付文档/带有空格 和逗号, 的目录/周期回顾.md'
 const notices = [
  {id: 900001, name: 'tasktrace.export.completed', notification: {filename: '周期回顾.md', path: savedPath}, read_at: null, created: new Date().toISOString()},
  {id: 900002, name: 'tasktrace.export.failed', notification: {filename: '失败导出.md', error: '无法写入：文件被占用，原文件已保留'}, read_at: null, created: new Date().toISOString()},
 ]
 await page.route('**/api/v1/notifications?*', route => route.fulfill({json: notices, headers: {'x-pagination-total-pages': '1'}}))
 await page.route('**/api/v1/notifications/900001', route => route.fulfill({json: {...notices[0], read_at: new Date().toISOString()}}))
 const archived = {...notices[0], id: 900003}
 await page.route('**/api/v2/tasktrace/notifications/history', route => route.fulfill({json: [{id: 'export-history', archived_at: new Date().toISOString(), notification: archived}]}))
 let archivedReveals = 0
 await page.route('**/api/v2/tasktrace/exports/900003/reveal', route => {archivedReveals++; return route.fulfill({status: 204})})
 let revealed = 0
 await page.route('**/api/v2/tasktrace/exports/900001/reveal', route => {revealed++; return route.fulfill({status: 204})})
 await page.goto('/')
 await page.locator('.notifications .trigger-button').click()
 const popup = page.locator('.notifications-list')
 await expect(popup).toContainText('导出成功：周期回顾.md')
 await expect(popup.locator('.export-path')).toHaveText('保存位置：' + savedPath)
 await expect(popup).toContainText('导出失败：失败导出.md')
 await expect(popup).toContainText('文件被占用，原文件已保留')
 await expect(popup.getByRole('link', {name: '打开所在文件夹'})).toHaveCount(1)
 // An immediate API response and its websocket echo must not create two entries.
 await page.evaluate(notice => window.dispatchEvent(new CustomEvent('tasktrace-export-notification', {detail: notice})), notices[0])
 await expect(popup.locator('.single-notification')).toHaveCount(2)
 await popup.getByRole('link', {name: '打开所在文件夹'}).click()
 await expect.poll(() => revealed).toBe(1)
 await expect(popup.locator('.single-notification').filter({hasText: '导出成功'}).locator('.read-indicator')).toHaveClass(/read/)
 await expect(popup.locator('.single-notification')).toHaveCount(2)
 await page.setViewportSize({width: 1280, height: 900})
 await popup.screenshot({path: testInfo.outputPath('export-notifications-desktop.png')})
 await page.setViewportSize({width: 390, height: 844})
 await popup.screenshot({path: testInfo.outputPath('export-notifications-mobile.png')})
 await expect.poll(() => popup.evaluate(element => element.scrollWidth <= element.clientWidth)).toBe(true)
 await popup.getByRole('button', {name: '历史通知', exact: true}).click()
 await expect(popup.locator('.notification-history .export-path')).toHaveText('保存位置：' + savedPath)
 await popup.getByRole('button', {name: /导出成功.*打开所在文件夹/}).click()
 await expect.poll(() => archivedReveals).toBe(1)
 await expect(popup.locator('.notification-history .export-path')).toBeVisible()
 await popup.screenshot({path: testInfo.outputPath('export-history-location-mobile.png')})
})
