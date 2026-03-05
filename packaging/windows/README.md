# Running blackbox-daemon as a Windows service

After completing the one-time setup (see below), the daemon can run as a Windows service that starts automatically on boot.

## One-time setup

Build and run the daemon interactively first to create the config file:

```powershell
go build -o blackbox-daemon.exe ./daemon
.\blackbox-daemon.exe
```

Answer the prompts (bastion URL, directory, token) and choose **y** to save the config.
The config is written to `%USERPROFILE%\.blackbox-daemon` (e.g. `C:\Users\You\.blackbox-daemon`) and is readable only by your user account.

---

## Option A: NSSM (recommended)

[NSSM](https://nssm.cc) wraps any executable as a Windows service with automatic restart and log capture.

1. Download NSSM and put `nssm.exe` somewhere on your PATH (e.g. `C:\Tools\`).

2. Open an **elevated** PowerShell (Run as Administrator):

```powershell
$binary = "C:\path\to\blackbox-daemon.exe"   # update this path

nssm install blackbox-daemon $binary
nssm set blackbox-daemon AppStdout "$env:USERPROFILE\.blackbox-daemon.log"
nssm set blackbox-daemon AppStderr "$env:USERPROFILE\.blackbox-daemon.log"
nssm set blackbox-daemon Start SERVICE_AUTO_START
nssm start blackbox-daemon
```

3. Check status:

```powershell
nssm status blackbox-daemon
```

To view logs:

```powershell
Get-Content "$env:USERPROFILE\.blackbox-daemon.log" -Wait
```

To remove:

```powershell
nssm stop blackbox-daemon
nssm remove blackbox-daemon confirm
```

---

## Option B: sc.exe (built-in, requires admin)

Windows' built-in service manager. No extra tools needed.

```powershell
$binary = "C:\path\to\blackbox-daemon.exe"   # update this path

sc.exe create blackbox-daemon binPath= $binary start= auto
sc.exe description blackbox-daemon "Blackbox file daemon"
sc.exe start blackbox-daemon
```

Check status:

```powershell
sc.exe query blackbox-daemon
```

To remove:

```powershell
sc.exe stop blackbox-daemon
sc.exe delete blackbox-daemon
```

> **Note:** `sc.exe` does not capture stdout/stderr. Logs will not be written unless you add a log wrapper. NSSM is recommended for easier log access.

---

## Config file location

The daemon reads its config from:

```
%USERPROFILE%\.blackbox-daemon
```

For example: `C:\Users\YourName\.blackbox-daemon`

The file contains `bastion_url`, `token`, and `hosted_path`. It is not stored in a system-wide location — if you run the service as a different user, run the interactive setup as that user first.
