# Install zen-octo on Windows, on arm64 or amd64.
#
#   irm https://raw.githubusercontent.com/praxis-labs-io/zen-octo/main/install.ps1 | iex
#
# INSTALL_DIR overrides where the binary lands, and defaults to
# $env:LOCALAPPDATA\Programs\zen-octo.
# VERSION pins a release, as v0.1.0, and defaults to the latest.
#
# This is install.sh for Windows. Change a decision in one and check the other.

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$repo = 'praxis-labs-io/zen-octo'
$binary = 'zen-octo.exe'

$installDir = if ($env:INSTALL_DIR) { $env:INSTALL_DIR } else {
	Join-Path $env:LOCALAPPDATA 'Programs\zen-octo'
}

# Not Write-Error: under $ErrorActionPreference Stop it throws, and 5.1 then doesn't reliably exit non-zero.
function Die($message) {
	[Console]::Error.WriteLine($message)
	exit 1
}

$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
	'AMD64' { 'amd64' }
	'ARM64' { 'arm64' }
	default { $env:PROCESSOR_ARCHITECTURE }
}
if ($arch -notin @('amd64', 'arm64')) {
	Die "No release binary for windows/$arch. Install it with Go instead:
    go install github.com/$repo/cmd/zen-octo@latest"
}

# Windows PowerShell 5.1 doesn't default to TLS 1.2, and GitHub refuses anything older.
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

if ($env:VERSION) {
	$tag = $env:VERSION
} else {
	try {
		$latest = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases/latest" `
			-Headers @{ 'User-Agent' = 'zen-octo-installer' } -UseBasicParsing
	} catch {
		$code = $null
		if ($_.Exception.PSObject.Properties['Response'] -and $_.Exception.Response) {
			$code = [int]$_.Exception.Response.StatusCode
		}
		switch ($code) {
			404 { Die 'There is no published release to install yet.' }
			403 { Die 'The GitHub API refused the lookup, most likely a rate limit. Retry, or set VERSION=vX.Y.Z.' }
			default { Die "Could not reach the GitHub API to look up the latest release. $($_.Exception.Message)" }
		}
	}
	$tag = if ($latest.PSObject.Properties['tag_name']) { $latest.tag_name } else { $null }
	if (-not $tag) { Die 'Could not read a tag out of the latest release.' }
}

$work = Join-Path ([IO.Path]::GetTempPath()) ("zen-octo-" + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $work -Force | Out-Null

try {
	$archive = "zen-octo_$($tag.TrimStart('v'))_windows_$($arch).zip"
	$download = "https://github.com/$repo/releases/download/$tag"
	$archivePath = Join-Path $work $archive
	$checksums = Join-Path $work 'checksums.txt'

	Write-Host "Downloading $tag for windows/$arch"
	try {
		Invoke-WebRequest -Uri "$download/$archive" -OutFile $archivePath -UseBasicParsing
	} catch {
		Die "Could not download $download/$archive"
	}

	try {
		Invoke-WebRequest -Uri "$download/checksums.txt" -OutFile $checksums -UseBasicParsing
	} catch {
		Die "Could not download the checksums for $tag."
	}

	$sum = (Get-FileHash -Path $archivePath -Algorithm SHA256).Hash.ToLower()
	$published = Get-Content $checksums |
		Where-Object { $_ -match "\s\*?$([regex]::Escape($archive))$" } |
		ForEach-Object { ($_ -split '\s+')[0].ToLower() } |
		Select-Object -First 1
	if (-not $published) {
		Die "$tag publishes no checksum for $archive. Nothing was installed."
	}
	if ($sum -ne $published) {
		Die "$archive does not match the checksum published for $tag. Nothing was installed."
	}

	Expand-Archive -Path $archivePath -DestinationPath $work -Force
	$staged = Join-Path $work $binary
	if (-not (Test-Path $staged)) {
		Die "$archive did not contain $binary."
	}

	New-Item -ItemType Directory -Path $installDir -Force | Out-Null
	$target = Join-Path $installDir $binary

	# Windows won't overwrite a running executable, which is what an upgrade is, so the old one moves aside.
	$retired = "$target.old"
	if (Test-Path $retired) {
		Remove-Item $retired -Force -ErrorAction SilentlyContinue
	}
	if (Test-Path $target) {
		Move-Item -Path $target -Destination $retired -Force
	}

	try {
		Move-Item -Path $staged -Destination $target -Force
	} catch {
		if (Test-Path $retired) { Move-Item -Path $retired -Destination $target -Force }
		Die "Could not write $target. $($_.Exception.Message)"
	}

	Write-Host "Installed $target"
} finally {
	Remove-Item -Recurse -Force $work -ErrorAction SilentlyContinue
}

if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
	Write-Host ''
	Write-Host 'git is not on PATH. zen-octo shells out to it for everything it reads.'
}

$paths = $env:PATH -split ';' | Where-Object { $_ }
if ($installDir -notin $paths) {
	Write-Host ''
	Write-Host "$installDir is not on your PATH. Add it:"
	Write-Host "    [Environment]::SetEnvironmentVariable('Path', `"`$env:PATH;$installDir`", 'User')"
	Write-Host 'Then open a new terminal.'
}
