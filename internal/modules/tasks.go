package modules

import (
	"encoding/json"
	"sync/atomic"

	"github.com/wittywilla/wittymediaserver/internal/tasks"
	"github.com/wittywilla/wittymodule"
)

// A helper can tell the server about work it's doing, with wittymodule.ReportTask, and it
// shows in the activity panel beside the server's own. A helper may be someone's fork, so
// each report is checked, and it can only ever speak about its own module's tasks: the
// task's name comes from the manifest, never from what the helper says.

var taskRegistry atomic.Pointer[tasks.Registry]

// ReportTasksTo says where helpers' task reports go. Until it's called they are ignored.
func ReportTasksTo(r *tasks.Registry) { taskRegistry.Store(r) }

// hearTasks sets a new connection up to hear task reports. It runs on the goroutine reading
// the helper, so nothing it does may block: Take only takes short locks.
func (p *proc) hearTasks(conn *wittymodule.Conn) {
	reg := taskRegistry.Load()
	if reg == nil {
		return
	}
	reporter := reg.ForSource(p.man.ID)
	p.reporter = reporter
	conn.OnNotify = func(method string, params json.RawMessage) {
		if method != wittymodule.NotifyTask {
			return
		}
		var t wittymodule.TaskReport
		if json.Unmarshal(params, &t) != nil || t.Validate() != nil {
			return
		}
		reporter.Take(tasks.Report{Key: t.Key, Title: t.Title, State: tasks.State(t.State), Done: t.Done, Total: t.Total, Item: t.Item})
	}
	// Reports only arrive during a call, unless something reads the pipe between calls. That
	// takes a helper that stays running and never streams, which says so with background.
	if p.keep && p.background {
		conn.Listen()
	}
}

// endTasks closes what a stopped helper left going: failed if it failed, and otherwise
// cancelled, since nobody can say it finished.
func (p *proc) endTasks(why string) {
	if p.reporter != nil {
		p.reporter.CloseAll(why == "failed")
		p.reporter = nil
	}
}
