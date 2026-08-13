# Builds TermDevTools for Windows and installs it together with its
# companion files (cat_columns.txt, endpoints.txt, cheatsheet.txt) into a
# self-contained directory.
#
# Run from anywhere; it locates the repository from its own path. Override
# the install location with $env:TERMDEVTOOLS_INSTALL_DIR if the default
# doesn't suit you.
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

$installDir = if ($env:TERMDEVTOOLS_INSTALL_DIR) { $env:TERMDEVTOOLS_INSTALL_DIR } else { "$env:LOCALAPPDATA\termdevtools" }
New-Item -ItemType Directory -Force -Path $installDir | Out-Null

Write-Host "Building termdevtools into $installDir ..."
$env:CGO_ENABLED = "0"
go build -trimpath -o "$installDir\termdevtools.exe" .

# cat_columns.txt/endpoints.txt are team-shared reference data (SPEC.md
# §9.1): always refreshed from the source tree. cheatsheet.txt is a personal
# starting point instead — only seeded once, never overwritten, so a
# previous customization survives re-running this script.
Copy-Item -Force cat_columns.txt, endpoints.txt -Destination $installDir
if (-not (Test-Path "$installDir\cheatsheet.txt")) {
	Copy-Item cheatsheet.txt.example "$installDir\cheatsheet.txt"
}

Write-Host ""
Write-Host "Installed to: $installDir"

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$installDir*") {
	Write-Host ""
	Write-Host "$installDir is not on your PATH yet. To add it for your user account, run:"
	Write-Host "  [Environment]::SetEnvironmentVariable('Path', `$env:Path + ';$installDir', 'User')"
	Write-Host "(then open a new terminal)"
	Write-Host ""
	Write-Host "Or just run it directly: $installDir\termdevtools.exe"
} else {
	Write-Host ""
	Write-Host "Run: termdevtools.exe"
}
