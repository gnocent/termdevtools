# Builds TermDevTools for Windows and installs the binary — all there is to
# install: its reference data and recipes are built in.
#
# Run from anywhere; it locates the repository from its own path. Override
# the install location with $env:TERMDEVTOOLS_INSTALL_DIR if the default
# doesn't suit you.
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

$installDir = if ($env:TERMDEVTOOLS_INSTALL_DIR) { $env:TERMDEVTOOLS_INSTALL_DIR } else { "$env:LOCALAPPDATA\termdevtools" }
New-Item -ItemType Directory -Force -Path $installDir | Out-Null

# What "termdevtools --version" will answer: the tag of the checkout, or how
# far past it this build is.
$version = "dev"
try {
	$described = git describe --tags --always --dirty 2>$null
	if ($LASTEXITCODE -eq 0 -and $described) { $version = $described }
} catch {}

Write-Host "Building termdevtools $version into $installDir ..."
$env:CGO_ENABLED = "0"
go build -trimpath -ldflags "-X main.version=$version" -o "$installDir\termdevtools.exe" .
if ($LASTEXITCODE -ne 0) { throw "go build failed" }

Write-Host ""
Write-Host "Installed to: $installDir"

# Versions up to 0.5 installed companion files next to the binary. Nothing is
# deleted here: endpoints.txt and cheatsheet.txt are still read, as the
# team's own additions (SPEC.md §9.1), and may have been customized.
$legacy = [ordered]@{
	"cat_columns.txt" = "no longer read, can be deleted"
	"endpoints.txt"   = "now built in; keep it only if you added endpoints of your own to it"
	"cheatsheet.txt"  = "still the editor's starting content; delete it to get the built-in one"
}
foreach ($name in $legacy.Keys) {
	if (Test-Path "$installDir\$name") {
		Write-Host "Left over from an earlier version: $installDir\$name ($($legacy[$name]))"
	}
}

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
