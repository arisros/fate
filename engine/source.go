package engine

import (
	"path"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/arisros/fate/action"
	"github.com/arisros/fate/describe"
)

type pkgAnchor struct{}

var (
	enginePkg = reflect.TypeFor[pkgAnchor]().PkgPath()
	rootPkg   = path.Dir(enginePkg)
)

var engineDirs = sync.OnceValue(func() [2]string {
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Dir(file)
	return [2]string{dir, filepath.Join(filepath.Dir(dir), "action")}
})

var readBuild = sync.OnceValue(func() *debug.BuildInfo {
	bi, _ := debug.ReadBuildInfo()
	return bi
})

// Sources reports where the machine and its guards and actions are written in Go source, for tooling that links a chart back to code.
func (m *Machine[Ctx, Evt]) Sources() describe.MachineSources {
	out := describe.MachineSources{Build: sourceBuild(), Machine: m.source}
	m.collectSources(m.root, &out)
	return out
}

func (m *Machine[Ctx, Evt]) collectSources(n *stateNode[Ctx, Evt], out *describe.MachineSources) {
	for _, child := range n.children {
		ss := describe.StateSources{
			Entry:  actionSources(child.entryActions),
			Exit:   actionSources(child.exitActions),
			OnDone: transitionSources(child.onDone),
		}
		for ev, ts := range child.on {
			if located := transitionSources(ts); located != nil {
				if ss.On == nil {
					ss.On = map[string][]describe.TransitionSources{}
				}
				ss.On[ev] = located
			}
		}
		if ss.Entry != nil || ss.Exit != nil || ss.OnDone != nil || ss.On != nil {
			if out.States == nil {
				out.States = map[string]describe.StateSources{}
			}
			out.States[dotPath(child)] = ss
		}
		m.collectSources(child, out)
	}
}

func transitionSources[Ctx any, Evt any](ts []TransitionConfig[Ctx, Evt]) []describe.TransitionSources {
	out := make([]describe.TransitionSources, len(ts))
	found := false
	for i, t := range ts {
		out[i] = describe.TransitionSources{Guard: funcLocation(t.Guard), Actions: actionSources(t.Actions)}
		found = found || out[i].Guard != nil || out[i].Actions != nil
	}
	if !found {
		return nil
	}
	return out
}

func actionSources[Ctx any, Evt any](actions []action.Action[Ctx, Evt]) []*describe.SourceLocation {
	out := make([]*describe.SourceLocation, len(actions))
	found := false
	for i, a := range actions {
		out[i] = actionLocation(a)
		found = found || out[i] != nil
	}
	if !found {
		return nil
	}
	return out
}

func actionLocation[Ctx any, Evt any](a action.Action[Ctx, Evt]) *describe.SourceLocation {
	for a != nil {
		w, ok := any(a).(interface {
			Unwrap() action.Action[Ctx, Evt]
		})
		if !ok {
			break
		}
		a = w.Unwrap()
	}
	if a == nil {
		return nil
	}
	if f, ok := any(a).(interface{ ImplFunc() any }); ok {
		return funcLocation(f.ImplFunc())
	}
	if apply, ok := reflect.TypeOf(a).MethodByName("Apply"); ok {
		return funcLocation(apply.Func.Interface())
	}
	return nil
}

func funcLocation(fn any) *describe.SourceLocation {
	v := reflect.ValueOf(fn)
	if !v.IsValid() || v.Kind() != reflect.Func || v.IsNil() {
		return nil
	}
	pc := v.Pointer()
	f := runtime.FuncForPC(pc)
	if f == nil {
		return nil
	}
	file, line := f.FileLine(pc)
	dirs, dir := engineDirs(), filepath.Dir(file)
	if !strings.HasSuffix(file, ".go") || ((dir == dirs[0] || dir == dirs[1]) && !strings.HasSuffix(file, "_test.go")) {
		return nil
	}
	return location(f.Name(), file, line)
}

func callerLocation() *describe.SourceLocation {
	pcs := make([]uintptr, 16)
	frames := runtime.CallersFrames(pcs[:runtime.Callers(2, pcs)])
	for {
		f, more := frames.Next()
		pkg := funcPackage(f.Function)
		own := (pkg == enginePkg || pkg == rootPkg) && !strings.HasSuffix(f.File, "_test.go")
		if f.Function != "" && !own {
			loc := location(f.Function, f.File, f.Line)
			loc.Func = ""
			return loc
		}
		if !more {
			return nil
		}
	}
}

func location(fn, file string, line int) *describe.SourceLocation {
	pkg := funcPackage(fn)
	name := strings.TrimPrefix(fn, pkg+".")
	if pkg == "main" {
		if bi := readBuild(); bi != nil && bi.Path != "" && bi.Path != "command-line-arguments" {
			pkg = bi.Path
		}
	}
	return &describe.SourceLocation{
		File: pkg + "/" + filepath.Base(file),
		Line: line,
		Func: path.Base(pkg) + "." + name,
		Path: file,
	}
}

func funcPackage(fn string) string {
	if i := strings.IndexByte(fn, '['); i >= 0 {
		fn = fn[:i]
	}
	slash := strings.LastIndexByte(fn, '/')
	dot := strings.IndexByte(fn[slash+1:], '.')
	if dot < 0 {
		return ""
	}
	return strings.ReplaceAll(fn[:slash+1+dot], "%2e", ".")
}

func sourceBuild() *describe.SourceBuild {
	bi := readBuild()
	if bi == nil || bi.Main.Path == "" {
		return nil
	}
	b := &describe.SourceBuild{Module: bi.Main.Path}
	if bi.Main.Version != "(devel)" {
		b.Version = bi.Main.Version
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			b.Revision = s.Value
		case "vcs.modified":
			b.Modified = s.Value == "true"
		}
	}
	return b
}
