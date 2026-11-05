//go:build windows

package main

import (
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"

	"github.com/wittywilla/wittymediaserver/internal/instance"
	"github.com/wittywilla/wittymediaserver/internal/logging"
)

// serviceName is what the installer registers the server as.
const serviceName = "WittyMediaServer"

// serviceFlag is how the service is registered: its first argument says it's one, rather than
// the program guessing from who started it.
const serviceFlag = "--windows-service"

// runAsService runs the server under Windows' service manager when it was registered that way,
// and says so. Otherwise the caller carries on as a normal program.
func runAsService(args []string) (int, bool) {
	if len(args) == 0 || args[0] != serviceFlag {
		return 0, false
	}
	asService = true
	h := &serviceHandler{args: args[1:]}
	if err := svc.Run(serviceName, h); err != nil {
		return 1, true
	}
	return h.code, true
}

type serviceHandler struct {
	args []string
	code int
}

func (h *serviceHandler) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}
	done := make(chan int, 1)
	go func() { done <- run(h.args) }()
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case code := <-done:
			h.code = code
			status <- svc.Status{State: svc.StopPending}
			if code == instance.ExitRunning {
				// Another server has the data folder. Said in Windows' own words, and the
				// restart rule (for crashes) doesn't apply to a service that stopped by itself.
				return false, uint32(windows.ERROR_SERVICE_ALREADY_RUNNING)
			}
			// A code that isn't zero tells the service manager to apply its restart rule.
			return false, uint32(code)
		case req := <-requests:
			switch req.Cmd {
			case svc.Interrogate:
				status <- req.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				if req.Cmd == svc.Shutdown {
					logging.NoteStop(logging.StopShutdown, "Windows told its services", "")
				} else {
					logging.NoteStop(logging.StopService, "the Windows service was stopped", "")
				}
				close(serviceStop)
				h.code = <-done
				return false, 0
			}
		}
	}
}
