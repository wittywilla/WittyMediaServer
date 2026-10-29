package cli

import (
	"fmt"
	"math"
	"strings"

	"github.com/wittywilla/wittydvd"
)

// crawlNode is an interactive screen reached by pressing a path of buttons.
type crawlNode struct {
	nav  *wittydvd.Nav
	step *wittydvd.Step
	pic  *wittydvd.MenuPicture
	path []int
}

// settleInteractive advances through non-interactive cells (intros, short
// clips, timed stills) until the viewer has something to press, the disc
// stops, or a long title starts playing.
func settleInteractive(o *wittydvd.Opened, nav *wittydvd.Nav, s *wittydvd.Step) (*wittydvd.Step, *wittydvd.MenuPicture, string) {
	for hop := 0; hop < 400; hop++ {
		if s.Kind == wittydvd.StepStop {
			return s, nil, "stop: " + s.Reason
		}
		pic, err := menuFor(o, nav, s)
		if err == nil && pic.Scrambled {
			return s, nil, "CSS-scrambled"
		}
		if err == nil && len(pic.Buttons) > 0 {
			return s, pic, ""
		}
		if s.Kind == wittydvd.StepTitle && s.Title != nil && s.Title.Duration > 600 {
			return s, nil, fmt.Sprintf("plays title %d (%s)", s.Title.TTN, hms(s.Title.Duration))
		}
		if math.IsInf(s.StillSec, 1) {
			return s, nil, "dead end: infinite still without buttons"
		}
		s = nav.CellEnded()
	}
	return s, nil, "gave up after 400 cells"
}

func screenKey(s *wittydvd.Step, regs *wittydvd.Registers) string {
	_ = regs
	return fmt.Sprintf("%s/%d/%d/%d", s.DomainName, s.VTS, s.PGCN, s.CellN)
}

func pathText(p []int) string {
	if len(p) == 0 {
		return "start"
	}
	parts := make([]string, len(p))
	for i, n := range p {
		parts[i] = fmt.Sprint(n)
	}
	return strings.Join(parts, ">")
}

// dvdCrawl presses every button of every reachable menu (breadth first) and
// reports where each one leads. It is the quickest way to find buttons that
// stop playback or menus that never show buttons.
func dvdCrawl(o *wittydvd.Opened, maxScreens int) int {
	nav := wittydvd.NewNav(o.Disc, wittydvd.NavOptions{})
	s, pic, why := settleInteractive(o, nav, nav.Start())
	if pic == nil {
		fmt.Println("first play:", describeStep(s), "→", why)
		return 1
	}
	queue := []crawlNode{{nav: nav, step: s, pic: pic}}
	seen := map[string]bool{screenKey(s, nav.Registers()): true}
	problems := 0
	for len(queue) > 0 && len(seen) <= maxScreens {
		cur := queue[0]
		queue = queue[1:]
		fmt.Printf("[%s] %s buttons=%d\n", pathText(cur.path), describeStep(cur.step), len(cur.pic.Buttons))
		for _, b := range cur.pic.Buttons {
			if b.Inert {
				continue
			}
			nx := cur.nav.Clone()
			c := b.Cmd
			st, npic, why := settleInteractive(o, nx, nx.PressButton(b.N, &c))
			path := append(append([]int{}, cur.path...), b.N)
			switch {
			case npic != nil:
				k := screenKey(st, nx.Registers())
				mark := ""
				if !seen[k] {
					seen[k] = true
					queue = append(queue, crawlNode{nav: nx, step: st, pic: npic, path: path})
					mark = " (new)"
				}
				fmt.Printf("   btn %2d %-40s → %s %s vts=%d pgc=%d cell=%d%s\n", b.N, b.CmdText, st.Kind, st.DomainName, st.VTS, st.PGCN, st.CellN, mark)
			default:
				flag := ""
				if strings.HasPrefix(why, "stop") || strings.HasPrefix(why, "dead") || strings.HasPrefix(why, "gave") {
					flag = "  <-- PROBLEM"
					problems++
				}
				fmt.Printf("   btn %2d %-40s → %s%s\n", b.N, b.CmdText, why, flag)
			}
		}
	}
	fmt.Printf("%d screens visited, %d problems\n", len(seen), problems)
	return 0
}
