package wittytuner

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Custom command: the owner's own program writes MPEG-TS to standard output.
// It runs a program on the server, so a web session alone cannot turn it on:
// the server must have been started with WMS_TUNER_COMMANDS=1. No shell is
// involved; the line is split into a program and its arguments.

func init() {
	register(Driver{
		ID: "command", Name: "Custom command", Status: "tested",
		Description: "Your own program writes MPEG-TS to standard output: for hardware or services nothing else here covers.",
		Fields: []Field{
			{Key: "command", Label: "Command", Kind: "text", Required: true, Help: "A program and its arguments. {number}, {name} and {id} become the channel's. No shell is used, so no pipes."},
			{Key: "channels", Label: "Channels", Kind: "textarea", Help: "One per line: number, name, id. Empty means one channel."},
			{Key: "streams", Label: "Streams at once", Kind: "number", Default: "1"},
		},
		build: func(c Config, e Env) (Source, error) {
			if !e.AllowCommands {
				return nil, errors.New("custom commands are off: start the server with WMS_TUNER_COMMANDS=1 to allow them")
			}
			args, err := SplitCommand(c.Options["command"])
			if err != nil {
				return nil, err
			}
			if len(args) == 0 {
				return nil, errors.New("fill in the command")
			}
			return &command{cfg: c, env: e, args: args}, nil
		},
	})
}

// SplitCommand splits a line into arguments, with single and double quotes
// and nothing else special.
func SplitCommand(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	in := false
	var q rune
	for _, c := range s {
		switch {
		case q != 0:
			if c == q {
				q = 0
			} else {
				cur.WriteRune(c)
			}
		case c == '"' || c == '\'':
			q, in = c, true
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			if in {
				out = append(out, cur.String())
				cur.Reset()
				in = false
			}
		default:
			cur.WriteRune(c)
			in = true
		}
	}
	if q != 0 {
		return nil, errors.New("a quote in the command is not closed")
	}
	if in {
		out = append(out, cur.String())
	}
	return out, nil
}

type command struct {
	cfg  Config
	env  Env
	args []string
}

func (c *command) Info() Info {
	i := Info{Driver: "command", Name: c.cfg.Name, Detail: c.args[0]}
	i.MaxStreams, _ = strconv.Atoi(c.cfg.Options["streams"])
	return i
}

func (c *command) Channels(context.Context) ([]Channel, error) {
	text := strings.TrimSpace(c.cfg.Options["channels"])
	if text == "" {
		return []Channel{{ID: "1", Number: "1", Name: firstNonEmpty(c.cfg.Name, "Custom")}}, nil
	}
	var out []Channel
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		f := strings.Split(line, ",")
		if len(f) < 2 {
			return nil, fmt.Errorf("%q should look like 5, News, news", line)
		}
		ch := Channel{Number: strings.TrimSpace(f[0]), Name: strings.TrimSpace(f[1])}
		ch.ID = ch.Number
		if len(f) > 2 {
			ch.ID = strings.TrimSpace(f[2])
		}
		out = append(out, ch)
	}
	return out, nil
}

func (c *command) Scan(ctx context.Context, _ ScanOptions, p func(Progress)) ([]Channel, error) {
	return noScan(c, ctx, p)
}

func (c *command) Close() error { return nil }

func (c *command) Open(ctx context.Context, ch Channel) (*Stream, error) {
	if !c.env.AllowCommands {
		return nil, errors.New("custom commands are off")
	}
	r := strings.NewReplacer("{number}", ch.Number, "{name}", ch.Name, "{id}", ch.ID)
	args := make([]string, 0, len(c.args)-1)
	for _, a := range c.args[1:] {
		args = append(args, r.Replace(a))
	}
	ctx, cancel := context.WithCancel(ctx)
	cmd := c.env.command(ctx, c.args[0], args...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("the command could not start: %w", err)
	}
	return guarded(&procReader{ReadCloser: out, cancel: cancel, wait: cmd.Wait}), nil
}
