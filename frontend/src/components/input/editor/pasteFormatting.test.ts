import {describe, expect, it} from 'vitest'
import {cleanPastedFormatting} from './pasteFormatting'

describe('external paste formatting', () => {
 it('removes Word fonts, colour and backgrounds but retains structure and indentation', () => {
  const result = cleanPastedFormatting('<p class="MsoNormal" style="font-size:40pt;color:white;background:black;margin-left:36pt;text-indent:12pt"><b>first</b><br>second</p><ul><li>item</li></ul>')
  expect(result).not.toMatch(/MsoNormal|font-size|color|background|<b>/)
  expect(result).toContain('margin-left: 36pt')
  expect(result).toContain('text-indent: 12pt')
  expect(result).toContain('first<br>second')
  expect(result).toContain('<ul><li>item</li></ul>')
 })
 it('retains interleaved images and table structure without active content', () => {
  const result = cleanPastedFormatting('<p>before<img src="/api/v2/tasks/1/attachments/2" onerror="bad()">after</p><table><tbody><tr><td colspan="2" style="background:red">cell</td></tr></tbody></table><script>bad()</script>')
  expect(result).toContain('before<img src="/api/v2/tasks/1/attachments/2">after')
  expect(result).toContain('colspan="2"')
  expect(result).not.toMatch(/bad|background|script/)
 })
})
