# Tests of the installers

Each proves a real install, upgrade and removal on a throwaway machine, with a before and after listing that has to match. None of them touch the machine they're started from.

| Script | Where it runs | What it proves |
|---|---|---|
| `installers-linux.sh` | anywhere Docker runs Linux containers | The `.deb` and `.rpm` on Debian, Ubuntu and Fedora, with systemd running: install, upgrade, remove, purge, `--uninstall`, nothing left, and taking over from a real 0.5.2-alpha |
| `check-msi.ps1` | Windows, changing nothing | Reads the MSI's tables and unpacks it with an administrative extract, which registers nothing |
| `installers-wine.sh` | anywhere Docker runs Linux containers | The MSI under Wine: install for one person and for everyone, upgrade, uninstall, with a before and after of the files and the registry |
| `installers-windows-sandbox.wsb` | Windows Sandbox, on a real Windows | The same as a person does it, on real Windows. Double-click it. The sandbox vanishes when it closes, and the results stay in the folder you started it from, as `results-windows.txt` |
| `installers-windows.ps1` | run by the sandbox | The script the sandbox runs. Refuses to run anywhere but a sandbox |

`internal/uninstall/uninstall_test.go` covers the same ground on every system with `go test`, with a fake registry and fake folders for each of Windows, macOS and Linux.
