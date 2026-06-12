# Running blackhaul-daemon as a Windows service

After completing the one-time setup (see below), the daemon can run as a Windows service that starts automatically on boot.

## One-time setup

Build and run the daemon interactively first to create the config file:

```powershell
go build -o blackhaul-daemon.exe ./daemon
.\blackhaul-daemon.exe
```

Answer the prompts (bastion URL, directory, token) and choose **y** to save the config.
The config is written to `%USERPROFILE%\.blackhaul-daemon` (e.g. `C:\Users\You\.blackhaul-daemon`) and is readable only by your user account.

---

## Option A: NSSM (recommended)

[NSSM](https://nssm.cc) wraps any executable as a Windows service with automatic restart and log capture.

1. Download NSSM and put `nssm.exe` somewhere on your PATH (e.g. `C:\Tools\`).

2. Open an **elevated** PowerShell (Run as Administrator):

```powershell
$binary = "C:\path\to\blackhaul-daemon.exe"   # update this path

nssm install blackhaul-daemon $binary
nssm set blackhaul-daemon AppStdout "$env:USERPROFILE\.blackhaul-daemon.log"
nssm set blackhaul-daemon AppStderr "$env:USERPROFILE\.blackhaul-daemon.log"
nssm set blackhaul-daemon Start SERVICE_AUTO_START
nssm start blackhaul-daemon
```

3. Check status:

```powershell
nssm status blackhaul-daemon
```

To view logs:

```powershell
Get-Content "$env:USERPROFILE\.blackhaul-daemon.log" -Wait
```

To remove:

```powershell
nssm stop blackhaul-daemon
nssm remove blackhaul-daemon confirm
```

---

## Option B: sc.exe (built-in, requires admin)

Windows' built-in service manager. No extra tools needed.

```powershell
$binary = "C:\path\to\blackhaul-daemon.exe"   # update this path

sc.exe create blackhaul-daemon binPath= $binary start= auto
sc.exe description blackhaul-daemon "Blackhaul file daemon"
sc.exe start blackhaul-daemon
```

Check status:

```powershell
sc.exe query blackhaul-daemon
```

To remove:

```powershell
sc.exe stop blackhaul-daemon
sc.exe delete blackhaul-daemon
```

> **Note:** `sc.exe` does not capture stdout/stderr. Logs will not be written unless you add a log wrapper. NSSM is recommended for easier log access.

---

## Config file location

The daemon reads its config from:

```
%USERPROFILE%\.blackhaul-daemon
```

For example: `C:\Users\YourName\.blackhaul-daemon`

The file contains `bastion_url`, `token`, and `hosted_path`. It is not stored in a system-wide location — if you run the service as a different user, run the interactive setup as that user first.
