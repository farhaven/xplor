// 2010 - Mathieu Lonjaret
// 2018 - Martin Kühl

// The xplor program is an acme files explorer, tree style.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"9fans.net/go/acme"
	"9fans.net/go/plan9"
	"9fans.net/go/plumb"
)

const (
	this     = "xplor"
	tag      = "Get All Up Cd Win Xplor "
	tab      = "\t"
	flagFile = " "
	flagLess = "▸"
	flagMore = "▾"
)

var (
	PLAN9 = os.Getenv("PLAN9")
	all   = flag.Bool("a", false, "Include directory entries whose names begin with a dot (.)")
	open  = map[string]bool{}
	win   *acme.Win
	root  string
)

func usage() {
	fmt.Fprintf(os.Stderr, "usage: %s [options] [path]\n", this)
	flag.PrintDefaults()
	os.Exit(2)
}

func main() {
	log.SetFlags(0)
	flag.Usage = usage
	flag.Parse()
	if err := setup(); err != nil {
		log.Fatal(err)
	}
	defer win.CloseFiles()
	for e := range win.EventChan() {
		if err := handle(e); err != nil {
			log.Print(err)
		}
	}
}

// Setup

func setup() error {
	if err := findRoot(); err != nil {
		return err
	}
	return openWindow()
}

func findRoot() error {
	switch flag.NArg() {
	case 0: // start at .
		var err error
		root, err = os.Getwd()
		return err
	case 1: // start at path
		var err error
		root = filepath.Clean(flag.Arg(0))
		if !filepath.IsAbs(root) {
			root, err = filepath.Abs(root)
			if err != nil {
				return err
			}
		}
		info, err := os.Stat(root)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("%s: Not a directory", root)
		}
		return nil
	default:
		usage()
		return nil
	}
}

func openWindow() error {
	var err error
	win, err = acme.New()
	if err != nil {
		return err
	}
	if err := win.Fprintf("tag", "%s", tag); err != nil {
		return err
	}
	if err := win.Ctl("dump %s", this); err != nil {
		return err
	}
	if err := win.Ctl("dumpdir %s", root); err != nil {
		return err
	}
	return draw()
}

// Drawing Structure

func draw() error {
	if err := win.Name("%s/-%s", root, this); err != nil {
		return err
	}
	var b bytes.Buffer
	if err := printRoot(&b); err != nil {
		return err
	}
	if _, err := win.Write("data", b.Bytes()); err != nil {
		return err
	}
	if err := win.Ctl("clean"); err != nil {
		return err
	}
	return selectEntry(root)
}

func redraw() error {
	win.Clear()
	return draw()
}

func redrawEntry(path string, info os.FileInfo, addr string, depth int) error {
	if err := selectEntryRegion(addr, depth); err != nil {
		return err
	}
	var b bytes.Buffer
	if err := printEntry(&b, path, info, depth); err != nil {
		return err
	}
	_, err := win.Write("data", b.Bytes())
	return err
}

// Directory Listing

func printRoot(w io.Writer) error {
	if err := printContents(w, root, 0); err != nil {
		return err
	}
	_, err := io.WriteString(w, "\n") // so acme can write after the last entry
	return err
}

func printContents(w io.Writer, dir string, depth int) error {
	infos, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, info := range infos {
		name := info.Name()
		path := filepath.Join(dir, name)
		if info.Mode()&os.ModeSymlink != 0 {
			if info, err = os.Stat(path); err != nil {
				return err
			}
		}
		if err := printEntry(w, path, info, depth); err != nil {
			return err
		}
	}
	return nil
}

func printEntry(w io.Writer, path string, info os.FileInfo, depth int) error {
	name := filepath.Base(path)
	if strings.HasPrefix(name, ".") && !*all {
		return nil
	}
	flag := flagFile
	if info.IsDir() {
		name += "/"
		flag = flagLess
		if open[path] {
			flag = flagMore
		}
	}
	tabs := strings.Repeat(tab, depth)
	if _, err := fmt.Fprintf(w, "%s %s%s\n", flag, tabs, name); err != nil {
		return err
	}
	if flag == flagMore {
		return printContents(w, path, depth+1)
	}
	return nil
}

// Buffer Interaction

func toggleAll() error {
	*all = !*all
	return redraw()
}

func goUp() error {
	root = filepath.Join(root, "..")
	return redraw()
}

func cd(dir string) error {
	root = dir
	return redraw()
}

// Line Interaction

func look(addr string) error {
	path, depth, err := abspath(addr)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return send(path)
	}
	open[path] = !open[path]
	if err := redrawEntry(path, info, addr, depth); err != nil {
		return err
	}
	return selectEntry(path)
}

func print(addr string) error {
	path, _, err := abspath(addr)
	if err != nil {
		return err
	}
	log.Print(path)
	return nil
}

// Selection

func selectEntry(path string) error {
	var steps strings.Builder
	steps.WriteString("0") // beginning
	for depth, name := range split(path) {
		tabs := strings.Repeat(tab, depth)
		fmt.Fprintf(&steps, "+/^..%s%s/", tabs, regexp.QuoteMeta(name)) // next entry
	}
	if err := win.Addr("%s", steps.String()); err != nil {
		return err
	}
	return win.Ctl("dot=addr\nshow")
}

func selectEntryRegion(addr string, depth int) error {
	var end strings.Builder // next line with at most depth tabs
	end.WriteString("^$")   // last line
	for i := 0; i <= depth; i++ {
		tabs := strings.Repeat(tab, i)
		fmt.Fprintf(&end, "|^..%s[^%s]", tabs, tab) // line with i tabs
	}
	return win.Addr("%s-/^/+/^/,%s+/%s/-/^/", addr, addr, end.String())
}

// Parsing Structure

// Determine entry name, depth
func entry(format string, args ...interface{}) (string, int, error) {
	fail := func(err error) (string, int, error) {
		return "", 0, err
	}
	if err := win.Addr(format, args...); err != nil {
		return fail(err)
	}
	buf, err := win.ReadAll("xdata")
	if err != nil {
		return fail(err)
	}
	line := strings.TrimSuffix(string(buf), "\n")
	if !strings.Contains(line, " ") {
		return fail(nil)
	}
	line = strings.SplitN(line, " ", 2)[1]
	name := strings.TrimLeft(line, tab)
	depth := (len(line) - len(name)) / len(tab)
	return name, depth, nil
}

// Determine absolute path, depth
func abspath(addr string) (string, int, error) {
	fail := func(err error) (string, int, error) {
		return "", 0, err
	}
	dir, depth, err := entry("%s-+", addr) // line containing addr
	if err != nil {
		return fail(err)
	}
	if dir == "" {
		return fail(nil)
	}
	for level := depth; level > 0; level-- {
		tabs := strings.Repeat(tab, level-1)
		parent, _, err := entry("%s-/^..%s[^%s]/-+", addr, tabs, tab) // line at depth before addr
		if err != nil {
			return fail(err)
		}
		dir = filepath.Join(parent, dir)
	}
	return filepath.Join(root, dir), depth, nil
}

// Determine components of path
func split(path string) []string {
	if path == root {
		return []string{}
	}
	path = strings.TrimPrefix(path, root+"/")
	return strings.Split(path, string(filepath.Separator))
}

// System Interaction

func send(path string) error {
	port, err := plumb.Open("send", plan9.OWRITE)
	if err != nil {
		return err
	}
	defer port.Close()
	msg := &plumb.Message{
		Src:  this,
		Dst:  "",
		Dir:  root,
		Type: "text",
		Data: []byte(path),
	}
	return msg.Send(port)
}

func run(exe, dir string) error {
	cmd := exec.Command(exe)
	cmd.Dir = dir
	return cmd.Start()
}

// Event Handling

func handle(e *acme.Event) error {
	switch e.C2 {
	// exec
	// - tag
	case 'x':
		switch string(e.Text) {
		case "Del":
			return win.Del(true)
		case "Get":
			return redraw()
		case "All":
			return toggleAll()
		case "Up":
			return goUp()
		case "Cd":
			dir, err := loc(e)
			if err != nil {
				return err
			}
			return cd(dir)
		case "Win":
			exe := filepath.Join(PLAN9, "bin", "win")
			dir, err := loc(e)
			if err != nil {
				return err
			}
			return run(exe, dir)
		case "Xplor":
			exe := this
			dir, err := loc(e)
			if err != nil {
				return err
			}
			return run(exe, dir)
		default:
			return win.WriteEvent(e)
		}
	// - body
	case 'X':
		if isComplex(e) {
			return win.WriteEvent(e)
		}
		return print(q(e))
	// look
	// - tag
	case 'l':
		return win.WriteEvent(e)
	// - body
	case 'L':
		if isComplex(e) {
			return win.WriteEvent(e)
		}
		return look(q(e))
	default:
		return nil
	}
}

func isComplex(e *acme.Event) bool {
	if e.OrigQ0 != e.OrigQ1 {
		// user has selected text
		return true
	}
	if e.Flag&8 != 0 {
		// user has chorded argument
		return true
	}
	return false
}

func q(e *acme.Event) string {
	return fmt.Sprintf("#%d", e.OrigQ0)
}

func loc(e *acme.Event) (string, error) {
	fail := func(err error) (string, error) {
		return "", err
	}
	if e.Flag&8 == 0 {
		return fail(nil)
	}
	loc := string(e.Loc)
	if loc == "" {
		return fail(nil)
	}
	prefix := fmt.Sprintf("%s/-%s:", root, this)
	if !strings.HasPrefix(loc, prefix) {
		return fail(nil)
	}
	loc = strings.TrimPrefix(loc, prefix)
	addrs := strings.SplitN(loc, ",", 2)
	dir, _, err := abspath(addrs[0])
	if err != nil {
		return fail(err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		return fail(err)
	}
	if info.IsDir() {
		return dir, nil
	}
	return filepath.Dir(dir), nil
}
