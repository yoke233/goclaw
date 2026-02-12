param(
    [switch]$WithLiveSubagent = $false
)

$ErrorActionPreference = "Stop"

Write-Host "== goclaw subagent E2E smoke =="
Write-Host ("Start: " + (Get-Date -Format "yyyy-MM-dd HH:mm:ss"))

Write-Host "`n[1/4] Run unit/integration tests"
go test ./agent/... ./cli/... ./memory/...
if ($LASTEXITCODE -ne 0) {
    throw "go test failed"
}

Write-Host "`n[2/4] Prepare isolated workspace"
$tempRoot = Join-Path $env:TEMP ("goclaw-e2e-" + [guid]::NewGuid().ToString("N"))
$workspace = Join-Path $tempRoot "workspace"
New-Item -ItemType Directory -Path $workspace -Force | Out-Null

# NOTE: loader currently uses prefix GOSKILLS_*
$env:GOSKILLS_WORKSPACE_PATH = $workspace

Write-Host "`n[3/4] Task command smoke"
$reqOutput = go run . task requirement --title "E2E requirement" --description "validate task workflow"
if ($LASTEXITCODE -ne 0) {
    throw "task requirement command failed"
}
Write-Host $reqOutput

$reqId = ""
foreach ($line in $reqOutput) {
    if ($line -match "ID:\s+(.+)$") {
        $reqId = $matches[1].Trim()
    }
}
if ([string]::IsNullOrWhiteSpace($reqId)) {
    throw "failed to parse requirement id from output"
}

$taskOutput = go run . task create --requirement $reqId --title "E2E task" --role frontend --acceptance "task can move to done"
if ($LASTEXITCODE -ne 0) {
    throw "task create command failed"
}
Write-Host $taskOutput

$taskId = ""
foreach ($line in $taskOutput) {
    if ($line -match "ID:\s+(.+)$") {
        $taskId = $matches[1].Trim()
    }
}
if ([string]::IsNullOrWhiteSpace($taskId)) {
    throw "failed to parse task id from output"
}

go run . task assign $taskId --role frontend --assignee "qa-user"
if ($LASTEXITCODE -ne 0) {
    throw "task assign command failed"
}

go run . task status $taskId --status doing --message "smoke started"
if ($LASTEXITCODE -ne 0) {
    throw "task status command failed"
}

go run . task progress $taskId --status doing --message "smoke progress entry"
if ($LASTEXITCODE -ne 0) {
    throw "task progress command failed"
}

go run . task status $taskId --status done --message "smoke finished"
if ($LASTEXITCODE -ne 0) {
    throw "task final status command failed"
}

$listOutput = go run . task list --requirement $reqId --with-progress --progress-limit 5
if ($LASTEXITCODE -ne 0) {
    throw "task list command failed"
}
Write-Host $listOutput

Write-Host "`n[4/4] Optional live subagent run"
if ($WithLiveSubagent) {
    Write-Host "Run manual sessions_spawn check from TUI/agent command (requires configured provider credentials)."
    Write-Host "Example prompt:"
    Write-Host '  请调用 sessions_spawn，task="[frontend] 生成一个登录页骨架"，task_id="<替换为上面的taskId>"'
} else {
    Write-Host "Skip live subagent run. Use -WithLiveSubagent to execute manual live validation."
}

Write-Host "`nE2E smoke passed."
Write-Host ("Workspace: " + $workspace)
Write-Host ("End: " + (Get-Date -Format "yyyy-MM-dd HH:mm:ss"))

