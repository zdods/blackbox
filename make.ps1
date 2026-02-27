# Cross-platform make-style script for Windows (PowerShell).
# Usage: .\make.ps1 <target>
# Targets: build-bastion, build-agent, up, dev

param(
    [Parameter(Mandatory = $true, Position = 0)]
    [ValidateSet("build-bastion", "build-agent", "up", "dev")]
    [string]$Target
)

$ErrorActionPreference = "Stop"

switch ($Target) {
    "build-bastion" {
        docker compose build bastion
    }
    "build-agent" {
        go build -o blackbox-agent.exe ./agent
    }
    "up" {
        docker compose up --build
    }
    "dev" {
        docker compose up --build --watch
    }
}
