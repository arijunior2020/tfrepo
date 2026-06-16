#Requires -Version 5.1
$ErrorActionPreference = "Stop"

$Repo       = "arijunior2020/tfrepo"
$Binary     = "tfrepo"
$InstallDir = if ($env:TFREPO_INSTALL_DIR) { $env:TFREPO_INSTALL_DIR }
              else { Join-Path $env:LOCALAPPDATA "Programs\tfrepo" }

# Detect arch
$Arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    "AMD64" { "amd64" }
    "ARM64" { "arm64" }
    default {
        Write-Error "Arquitetura não suportada: $($env:PROCESSOR_ARCHITECTURE)"
        exit 1
    }
}

# Resolve latest version via GitHub API
Write-Host "→ Buscando versão mais recente..."
$Release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest" -UseBasicParsing
$Version  = $Release.tag_name -replace "^v", ""
Write-Host "→ Versão: v$Version"

# Build URL
$Filename = "${Binary}_${Version}_windows_${Arch}.zip"
$Url      = "https://github.com/$Repo/releases/download/v$Version/$Filename"

# Download and extract in temp dir
$Tmp = Join-Path $env:TEMP "tfrepo-install-$(Get-Random)"
New-Item -ItemType Directory -Path $Tmp -Force | Out-Null
try {
    Write-Host "→ Baixando $Url..."
    $ZipPath = Join-Path $Tmp $Filename
    Invoke-WebRequest -Uri $Url -OutFile $ZipPath -UseBasicParsing
    Expand-Archive -Path $ZipPath -DestinationPath $Tmp -Force

    # Install binary
    if (-not (Test-Path $InstallDir)) {
        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    }
    Copy-Item -Path (Join-Path $Tmp "$Binary.exe") -Destination $InstallDir -Force

    # Add to user PATH if not already present
    $UserPath = [Environment]::GetEnvironmentVariable("PATH", "User") ?? ""
    if ($UserPath -notlike "*$InstallDir*") {
        [Environment]::SetEnvironmentVariable("PATH", "$UserPath;$InstallDir", "User")
        Write-Host "→ $InstallDir adicionado ao PATH do usuário."
        Write-Host "  Reinicie o terminal para aplicar."
    }
} finally {
    Remove-Item -Recurse -Force $Tmp -ErrorAction SilentlyContinue
}

Write-Host ""
Write-Host "✓ tfrepo instalado em $InstallDir\$Binary.exe"
& "$InstallDir\$Binary.exe" --version
