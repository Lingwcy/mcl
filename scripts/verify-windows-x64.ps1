param(
    [Parameter(Mandatory = $true)][string]$Application,
    [Parameter(Mandatory = $true)][string]$Installer
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

# NSIS itself can be PE32/i386 even when the installed application is x64.
# Check the real application and the payload, not the installer stub or name.
function Assert-X64Application([string]$Path) {
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
        if ($machine -ne 0x8664 -or $magic -ne 0x20b) {
            throw ("Expected AMD64/x86-64 PE32+, found machine=0x{0:x4}, magic=0x{1:x4}: {2}" -f $machine, $magic, $Path)
        }
    } finally {
        $reader.Dispose()
    }

    $info = & go version -m $Path
    if ($LASTEXITCODE -ne 0) { throw "Cannot read Go build information: $Path" }
    $info = $info -join "`n"
    if ($info -notmatch "(?m)^\s*build\s+GOOS=windows\s*$" -or
        $info -notmatch "(?m)^\s*build\s+GOARCH=amd64\s*$") {
        throw "Go build information is not windows/amd64: $Path"
    }
    Write-Host "Verified $Path : AMD64 (0x8664), PE32+, GOOS=windows, GOARCH=amd64"
}

Assert-X64Application $Application
$temporary = Join-Path ([System.IO.Path]::GetTempPath()) ("mcl-pe-" + [guid]::NewGuid().ToString("N"))
try {
    $sevenZip = (Get-Command 7z -CommandType Application -ErrorAction Stop).Source
    & $sevenZip x $Installer "-o$temporary" -y | Out-Host
    if ($LASTEXITCODE -ne 0) { throw "Cannot extract the NSIS payload for inspection" }
    $payloads = @(Get-ChildItem -LiteralPath $temporary -Recurse -File -Filter mcl.exe)
    if ($payloads.Count -ne 1) { throw "Expected exactly one mcl.exe in the installer payload" }
    $payload = $payloads[0].FullName
    Assert-X64Application $payload
    $builtHash = (Get-FileHash -LiteralPath $Application -Algorithm SHA256).Hash
    $payloadHash = (Get-FileHash -LiteralPath $payload -Algorithm SHA256).Hash
    if ($builtHash -ne $payloadHash) { throw "Packaged mcl.exe does not match the verified build" }
    if ($env:GITHUB_STEP_SUMMARY) {
        @(
            "### Windows architecture verified",
            "- Target: x64 / x86-64 / AMD64 (64-bit Intel and AMD Windows).",
            "- Built and packaged mcl.exe: PE machine 0x8664, PE32+, Go windows/amd64.",
            "- Matching application SHA-256: $($builtHash.ToLowerInvariant())",
            "- NSIS was extracted for static inspection; neither the installer nor the app was executed.",
            "- ARM64 and 32-bit x86 are not this build target."
        ) | Add-Content -LiteralPath $env:GITHUB_STEP_SUMMARY
    }
} finally {
    if (Test-Path -LiteralPath $temporary) { Remove-Item -LiteralPath $temporary -Recurse -Force }
}
