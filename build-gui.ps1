$ErrorActionPreference = 'Stop'
Push-Location $PSScriptRoot
try {
    Push-Location (Join-Path $PSScriptRoot 'frontend')
    try {
        & npm.cmd ci --no-fund --no-audit
        if ($LASTEXITCODE -ne 0) { throw 'Installation frontend interrompue.' }
        & npm.cmd run build
        if ($LASTEXITCODE -ne 0) { throw 'Compilation frontend interrompue.' }
    } finally {
        Pop-Location
    }
    & go build -tags production -ldflags '-H windowsgui' -o deskctl-gui.exe .\cmd\deskctl-gui
    if ($LASTEXITCODE -ne 0) { throw 'Compilation graphique interrompue.' }
    Write-Output 'Application portable : deskctl-gui.exe'
} finally {
    Pop-Location
}
