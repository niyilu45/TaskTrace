$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
[Windows.Forms.Application]::EnableVisualStyles()
$settingsFile = Join-Path $PSScriptRoot 'tasktrace-settings.json'
$form = New-Object Windows.Forms.Form
$form.Text = 'TaskTrace 数据目录配置'
$form.Size = New-Object Drawing.Size(560,280)
$form.StartPosition = 'CenterScreen'
$form.FormBorderStyle = 'FixedDialog'
$form.MaximizeBox = $false
$form.TopMost = $true
$form.Font = New-Object Drawing.Font('Microsoft YaHei UI',9)
$label = New-Object Windows.Forms.Label
$label.Text = '数据保存目录（数据库、图片、会话和悬浮窗设置）'
$label.SetBounds(18,18,510,25)
$inputPath = New-Object Windows.Forms.TextBox
$inputPath.SetBounds(18,50,405,28)
$inputPath.Text = 'data'
try {
    if (Test-Path -LiteralPath $settingsFile) {
        $current = [IO.File]::ReadAllText($settingsFile) | ConvertFrom-Json
        $inputPath.Text = [string]$current.dataDirectory
    }
} catch { [void][Windows.Forms.MessageBox]::Show('原配置格式有误，请重新选择目录后保存。','TaskTrace') }
$browse = New-Object Windows.Forms.Button
$browse.Text = '选择…'
$browse.SetBounds(432,48,95,30)
$browse.Add_Click({
    $picker = New-Object Windows.Forms.FolderBrowserDialog
    $picker.Description = '选择 TaskTrace 数据保存目录'
    try { if ($picker.ShowDialog($form) -eq 'OK') { $inputPath.Text = $picker.SelectedPath } } finally { $picker.Dispose() }
})
$help = New-Object Windows.Forms.Label
$help.Text = "支持绝对路径和相对于程序目录的路径，默认 data。`n保存后需退出并重新启动 TaskTrace。`n切换目录不会搬移或覆盖原数据。要继续使用旧数据，请先退出程序，`n将旧数据目录的全部内容复制到新目录，再启动。"
$help.SetBounds(18,92,510,92)
$save = New-Object Windows.Forms.Button
$save.Text = '保存配置'
$save.SetBounds(410,190,115,32)
$save.Add_Click({
    try {
        $configured = $inputPath.Text.Trim()
        if ([string]::IsNullOrWhiteSpace($configured)) { throw '请填写数据目录。' }
        $resolved = [Environment]::ExpandEnvironmentVariables($configured)
        if (![IO.Path]::IsPathRooted($resolved)) { $resolved = Join-Path $PSScriptRoot $resolved }
        $resolved = [IO.Path]::GetFullPath($resolved)
        if (Test-Path -LiteralPath $resolved -PathType Leaf) { throw '此路径是文件，请选择文件夹。' }
        $settings = [ordered]@{ dataDirectory = $configured }
        if (Test-Path -LiteralPath $settingsFile) {
            try {
                $old = [IO.File]::ReadAllText($settingsFile) | ConvertFrom-Json
                foreach ($property in $old.PSObject.Properties) { if ($property.Name -ne 'dataDirectory') { $settings[$property.Name] = $property.Value } }
            } catch { }
        }
        [IO.File]::WriteAllText($settingsFile, ($settings | ConvertTo-Json -Depth 10), [Text.UTF8Encoding]::new($false))
        [void][Windows.Forms.MessageBox]::Show(('已保存。重启后使用：' + $resolved),'TaskTrace')
        $form.Close()
    } catch { [void][Windows.Forms.MessageBox]::Show($_.Exception.Message,'无法保存配置') }
})
$form.Controls.AddRange(@($label,$inputPath,$browse,$help,$save))
[void]$form.ShowDialog()
$form.Dispose()