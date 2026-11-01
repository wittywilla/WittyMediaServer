package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

// DB is what the history needs from the server's database.
type DB interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// SQLStore keeps the history in the task_history table, with a limit on how much and
// for how long. Writes are small, and arrive from the registry's own writer.
type SQLStore struct {
	DB DB
	// Keep says how many finished tasks to hold, and for how many days. Read when
	// pruning, so a change in Settings applies from the next task.
	Keep func() (count, days int)
	// Failed is called when a write fails. Losing a history row isn't worth more than a log line.
	Failed func(err error)
}

const (
	defaultKeepCount = 500
	defaultKeepDays  = 30
)

const taskColumns = `id, type, title, state, phase, library_id, library_name, done, total, unit, started_at, updated_at, finished_at,
	by_id, by_name, source, result, error, fix, detail, problem_count, problems, retry_key, private, admin_only`

func (s *SQLStore) fail(err error) {
	if err != nil && s.Failed != nil {
		s.Failed(err)
	}
}

// Begin notes a task as it starts, so a crash can be told about afterwards.
func (s *SQLStore) Begin(t Task) { s.write(t) }

// End saves a task as it ended, and trims the history.
func (s *SQLStore) End(t Task) {
	s.write(t)
	s.prune()
}

func (s *SQLStore) write(t Task) {
	ctx := context.Background()
	problems := ""
	if len(t.Problems) > 0 {
		if b, err := json.Marshal(t.Problems); err == nil {
			problems = string(b)
		}
	}
	_, err := s.DB.ExecContext(ctx, `INSERT OR REPLACE INTO task_history(`+taskColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.ID, t.Type, t.Title, string(t.State), t.Phase, t.LibraryID, t.LibraryName, t.Done, t.Total, t.Unit, t.StartedAt, t.UpdatedAt, t.FinishedAt,
		t.ByID, t.By, t.Source, t.Result, t.Error, t.Fix, t.Detail, t.ProblemCount, problems, t.RetryKey, boolInt(t.Private), boolInt(t.AdminOnly))
	s.fail(err)
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (s *SQLStore) prune() {
	count, days := defaultKeepCount, defaultKeepDays
	if s.Keep != nil {
		count, days = s.Keep()
	}
	ctx := context.Background()
	if days > 0 {
		_, err := s.DB.ExecContext(ctx, `DELETE FROM task_history WHERE finished_at > 0 AND finished_at < ?`, time.Now().AddDate(0, 0, -days).Unix())
		s.fail(err)
	}
	if count > 0 {
		_, err := s.DB.ExecContext(ctx, `DELETE FROM task_history WHERE finished_at > 0 AND id NOT IN (SELECT id FROM task_history WHERE finished_at > 0 ORDER BY finished_at DESC, rowid DESC LIMIT ?)`, count)
		s.fail(err)
	}
}

// MarkInterrupted says what happened to tasks the last run of the server left going.
// Call it once, before anything starts.
func (s *SQLStore) MarkInterrupted(ctx context.Context) {
	now := time.Now().Unix()
	_, err := s.DB.ExecContext(ctx, `UPDATE task_history SET state = ?, error = ?, fix = ?, finished_at = ?, updated_at = ?, phase = '' WHERE state IN (?, ?, ?)`,
		string(Failed), "The server stopped while this was going on.", "Start it again if it's still needed.", now, now, string(Queued), string(Running), string(Paused))
	s.fail(err)
}

// Query narrows a look through the history.
type Query struct {
	Limit   int
	Before  int64 // finished before this time
	Type    string
	State   string
	Library int64
}

// List reads the history, newest first.
func (s *SQLStore) List(ctx context.Context, q Query) ([]Task, error) {
	if q.Limit <= 0 || q.Limit > 200 {
		q.Limit = 50
	}
	where := []string{"finished_at > 0"}
	var args []any
	if q.Before > 0 {
		where = append(where, "finished_at < ?")
		args = append(args, q.Before)
	}
	if q.Type != "" {
		where = append(where, "type = ?")
		args = append(args, q.Type)
	}
	if q.State != "" {
		where = append(where, "state = ?")
		args = append(args, q.State)
	}
	if q.Library != 0 {
		where = append(where, "library_id = ?")
		args = append(args, q.Library)
	}
	args = append(args, q.Limit)
	rows, err := s.DB.QueryContext(ctx, `SELECT `+taskColumns+` FROM task_history WHERE `+strings.Join(where, " AND ")+` ORDER BY finished_at DESC, rowid DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Task{}
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Get reads one task of the history.
func (s *SQLStore) Get(ctx context.Context, id string) (Task, bool) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+taskColumns+` FROM task_history WHERE id = ?`, id)
	if err != nil {
		return Task{}, false
	}
	defer rows.Close()
	if !rows.Next() {
		return Task{}, false
	}
	t, err := scanTask(rows)
	return t, err == nil
}

func scanTask(rows *sql.Rows) (Task, error) {
	var t Task
	var state, problems string
	var private, adminOnly int
	if err := rows.Scan(&t.ID, &t.Type, &t.Title, &state, &t.Phase, &t.LibraryID, &t.LibraryName, &t.Done, &t.Total, &t.Unit, &t.StartedAt, &t.UpdatedAt, &t.FinishedAt,
		&t.ByID, &t.By, &t.Source, &t.Result, &t.Error, &t.Fix, &t.Detail, &t.ProblemCount, &problems, &t.RetryKey, &private, &adminOnly); err != nil {
		return t, err
	}
	t.State = State(state)
	t.Private, t.AdminOnly = private != 0, adminOnly != 0
	t.Percent = -1
	if t.Total > 0 {
		t.Percent = min(100, float64(t.Done)*100/float64(t.Total))
	}
	if problems != "" {
		_ = json.Unmarshal([]byte(problems), &t.Problems)
	}
	return t, nil
}

// Clear forgets the history, or one task of it. What's still going stays.
func (s *SQLStore) Clear(ctx context.Context, id string) error {
	if id != "" {
		_, err := s.DB.ExecContext(ctx, `DELETE FROM task_history WHERE id = ? AND finished_at > 0`, id)
		return err
	}
	_, err := s.DB.ExecContext(ctx, `DELETE FROM task_history WHERE finished_at > 0`)
	return err
}
