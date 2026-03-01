# Cross-platform make-style script for Windows (PowerShell).
# Usage: .\make.ps1 <target>
# Targets: build-bastion, build-daemon, up, dev

param(
    [Parameter(Mandatory = $true, Position = 0)]
    [ValidateSet("build-bastion", "build-daemon", "up", "dev")]
    [string]$Target
)

$ErrorActionPreference = "Stop"

switch ($Target) {
    "build-bastion" {
        docker compose build bastion
    }
    "build-daemon" {
        go build -o blackbox-daemon.exe ./daemon
    }
    "up" {
        docker compose up --build
    }
    "dev" {
        docker compose up --build --watch
    }
}
