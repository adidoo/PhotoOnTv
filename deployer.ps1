# Construit, signe et installe l'appli TV sur la télé, puis la lance.
# Usage : .\deployer.ps1 [-Dossier tv] [-Inspecteur]   (-Inspecteur : lance avec l'inspecteur, affiche le port)
# Réglages propres à l'installation : deployer.local.ps1 (voir deployer.local.example.ps1).
param([string]$Dossier = 'tv', [switch]$Inspecteur)

$tizen  = 'C:\tizen-studio\tools\ide\bin\tizen.bat'
$sdb    = 'C:\tizen-studio\tools\sdb.exe'
$tv     = $env:PHOTOONTV_TV        # IP de la télé
$cible  = $env:PHOTOONTV_CIBLE     # nom de la cible donné par « sdb devices »
$profil = $env:PHOTOONTV_PROFIL    # profil de certificat Samsung
$local  = Join-Path $PSScriptRoot 'deployer.local.ps1'
if (Test-Path $local) { . $local }
if (-not $tv -or -not $cible -or -not $profil) { throw 'Renseigner $tv, $cible et $profil dans deployer.local.ps1' }

$src = Join-Path $PSScriptRoot $Dossier
$out = Join-Path $PSScriptRoot 'build'
if (-not (Test-Path (Join-Path $src 'js\config.js'))) { throw "Créer $Dossier\js\config.js à partir de config.example.js" }

& $sdb connect $tv | Out-Null
# Identifiant de l'appli lu dans config.xml
$appId = ([xml](Get-Content (Join-Path $src 'config.xml') -Encoding UTF8)).widget.application.id

& $tizen package -t wgt -s $profil -o $out -- $src | Select-String 'Package File|rror'
$wgt = Get-ChildItem $out -Filter *.wgt | Sort-Object LastWriteTime -Descending | Select-Object -First 1
# L'installeur de la télé refuse les noms de fichier avec espaces
$nom = 'photoontv.wgt'
Move-Item -Force $wgt.FullName (Join-Path $out $nom)
& $tizen install -n $nom -t $cible -- $out | Select-String 'success|Fail|rror'

if ($Inspecteur) {
    $r = (& $sdb -s "${tv}:26101" shell 0 debug $appId) -join ' '
    $port = ([regex]::Match($r, 'port:\s*(\d+)')).Groups[1].Value
    & $sdb -s "${tv}:26101" forward "tcp:$port" "tcp:$port" | Out-Null
    "Inspecteur : http://127.0.0.1:$port/json"
} else {
    & $tizen run -p $appId -t $cible | Select-String 'launch|rror'
}
