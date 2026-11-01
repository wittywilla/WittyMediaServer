//go:build windows

package main

import (
	"golang.org/x/sys/windows/svc"
)

// serviceName is what the installer registers the server as.
const serviceName = "WittyMediaServer"

// runAsService runs the server under Windows' service manager when that's who started it,
// and says so. Otherwise the caller carries on as a normal program.
func runAsService(args []string) (int, bool) {
	isService, err := svc.IsWindowsService()
	if err != nil || !isService {
		return 0, false
	}
	h := &serviceHandler{args: args}
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
			// A code that isn't zero tells the service manager to apply its restart rule.
			return false, uint32(code)
		case req := <-requests:
			switch req.Cmd {
			case svc.Interrogate:
				status <- req.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				close(serviceStop)
				h.code = <-done
				return false, 0
			}
		}
	}
}
