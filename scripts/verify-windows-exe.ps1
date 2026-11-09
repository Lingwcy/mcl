param(
    [Parameter(Mandatory = $true)][string]$Application,
    [Parameter(Mandatory = $true)][ValidateSet("amd64", "arm64")][string]$Arch,
    # The update archive holds the same mcl.exe that the release publishes.
    [string]$Archive
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$machines = @{ amd64 = 0x8664; arm64 = 0xAA64 }

# Check the real PE header and Go build information, not the file name.
function Assert-WindowsApplication([string]$Path) {
    $stream = [System.IO.File]::OpenRead((Resolve-Path -LiteralPath $Path).Path)
    $reader = [System.IO.BinaryReader]::new($stream)
    try {
        if ($stream.Length -lt 64 -or $reader.ReadUInt16() -ne 0x5a4d) {
            throw "Not a DOS/PE executable: $Path"
        }
        $stream.Position = 0x3c
        $offset = $reader.ReadUInt32()
        if ([long]$offset + 26 -gt $stream.Length) {
            throw "Invalid PE header offset: $Path"
        }
        $stream.Position = $offset
        if ($reader.ReadUInt32() -ne 0x00004550) {
            throw "Missing PE signature: $Path"
        }
        $machine = $reader.ReadUInt16()
        $stream.Position = [long]$offset + 24
        $magic = $reader.ReadUInt16()
        if ($machine -ne $machines[$Arch] -or $magic -ne 0x20b) {
            throw ("Expected {0} PE32+ (machine=0x{1:x4}), found machine=0x{2:x4}, magic=0x{3:x4}: {4}" -f $Arch, $machines[$Arch], $machine, $magic, $Path)
        }
    } finally {
        $reader.Dispose()
    }

    $info = & go version -m $Path
    if ($LASTEXITCODE -ne 0) { throw "Cannot read Go build information: $Path" }
    $info = $info -join "`n"
    if ($info -notmatch "(?m)^\s*build\s+GOOS=windows\s*$" -or
        $info -notmatch "(?m)^\s*build\s+GOARCH=$Arch\s*$") {
        throw "Go build information is not windows/${Arch}: $Path"
    }
    Write-Host ("Verified {0} : {1} (0x{2:x4}), PE32+, GOOS=windows, GOARCH={1}" -f $Path, $Arch, $machines[$Arch])
}

Assert-WindowsApplication $Application
$builtHash = (Get-FileHash -LiteralPath $Application -Algorithm SHA256).Hash
$summary = @(
    "### Windows $Arch executable verified",
    "- mcl.exe: PE machine 0x$('{0:x4}' -f $machines[$Arch]), PE32+, Go windows/$Arch.",
    "- SHA-256: $($builtHash.ToLowerInvariant())"
)

if ($Archive) {
    $temporary = Join-Path ([System.IO.Path]::GetTempPath()) ("mcl-update-" + [guid]::NewGuid().ToString("N"))
    New-Item -ItemType Directory -Path $temporary | Out-Null
    try {
        tar -xzf $Archive -C $temporary
        if ($LASTEXITCODE -ne 0) { throw "Cannot extract the update archive: $Archive" }
        $payload = Join-Path $temporary "mcl.exe"
        if (-not (Test-Path -LiteralPath $payload)) { throw "The update archive has no mcl.exe at its root" }
        Assert-WindowsApplication $payload
        $payloadHash = (Get-FileHash -LiteralPath $payload -Algorithm SHA256).Hash
        if ($builtHash -ne $payloadHash) { throw "The update archive's mcl.exe does not match the released executable" }
        $summary += "- The update archive holds the same mcl.exe."
    } finally {
        Remove-Item -LiteralPath $temporary -Recurse -Force -ErrorAction SilentlyContinue
    }
}

if ($env:GITHUB_STEP_SUMMARY) {
    $summary | Add-Content -LiteralPath $env:GITHUB_STEP_SUMMARY
}
