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
	"strings"

	"9fans.net/go/acme"
	"9fans.net/go/plan9"
	"9fans.net/go/plumb"
)

const (
	this     = "xplor"
	tag      = "Get All Up Win Xplor "
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
		root = filepath.Clean(flag.Arg(0))
		if filepath.IsAbs(root) {
			return nil
		}
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		root = filepath.Join(cwd, root)
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
	if err := win.Name("%s-%s", this, root); err != nil {
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
	return focus(root)
}

func redraw() error {
	win.Clear()
	return draw()
}

func redrawDir(path, addr string, depth int) error {
	if open[path] {
		return insertDir(path, addr, depth)
	} else {
		return removeDir(path, addr, depth)
	}
}

// Directory Listing

func printRoot(w io.Writer) error {
	return printDir(w, root, 0)
}

func printDir(w io.Writer, dir string, depth int) error {
	infos, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}
	tabs := strings.Repeat(tab, depth)
	for _, info := range infos {
		name := info.Name()
		if strings.HasPrefix(name, ".") && !*all {
			continue
		}
		path := filepath.Join(dir, name)
		flag := flagFile
		if info.IsDir() {
			flag = flagLess
			if open[path] {
				flag = flagMore
			}
		}
		if _, err := fmt.Fprintf(w, "%s %s%s\n", flag, tabs, name); err != nil {
			return err
		}
		if flag == flagMore {
			if err := printDir(w, path, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func insertDir(path, addr string, depth int) error {
	if err := win.Addr("%s+/^/", addr); err != nil {
		return err
	}
	var b bytes.Buffer
	if err := printDir(&b, path, depth+1); err != nil {
		return err
	}
	_, err := win.Write("data", b.Bytes())
	return err
}

func removeDir(path, addr string, depth int) error {
	tabs := strings.Repeat(tab, depth)
	if err := win.Addr("%s+/^/,%s+/^..%s[^%s]/-/^/", addr, addr, tabs, tab); err != nil {
		return err
	}
	_, err := win.Write("data", nil)
	return err
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
	if err := redrawDir(path, addr, depth); err != nil {
		return err
	}
	return focus(path)
}

func focus(path string) error {
	path, err := filepath.Rel(root, path)
	if err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("#0")
	for depth, name := range split(path) {
		tabs := strings.Repeat(tab, depth)
		fmt.Fprintf(&b, "+/^..%s%s/", tabs, name)
	}
	if err := win.Addr("%s", b.String()); err != nil {
		return err
	}
	return win.Ctl("dot=addr\nshow")
}

func print(addr string) error {
	path, _, err := abspath(addr)
	if err != nil {
		return err
	}
	log.Print(path)
	return nil
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
	var part string
	parts := make([]string, 0)
	for path != "" && path != "." {
		path, part = filepath.Split(path)
		path = filepath.Clean(path)
		parts = append([]string{part}, parts...)
	}
	return parts
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
	prefix := fmt.Sprintf("%s-%s:", this, root)
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
