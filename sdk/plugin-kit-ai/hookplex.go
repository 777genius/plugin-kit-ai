package pluginkitai

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/777genius/plugin-kit-ai/sdk/claude"
	"github.com/777genius/plugin-kit-ai/sdk/codex"
	"github.com/777genius/plugin-kit-ai/sdk/internal/descriptors/gen"
	"github.com/777genius/plugin-kit-ai/sdk/internal/runtime"
	"github.com/777genius/plugin-kit-ai/sdk/internal/runtime/process"
)

type IO = runtime.IO
type Env = runtime.Env
type Logger = runtime.Logger
type Middleware = runtime.Middleware
type Next = runtime.Next
type Handled = runtime.Handled
type InvocationContext = runtime.InvocationContext
type Result = runtime.Result
type NopLogger = runtime.NopLogger
type CapabilityID = runtime.CapabilityID
type SupportStatus = runtime.SupportStatus
type MaturityLevel = runtime.MaturityLevel
type TransportMode = runtime.TransportMode
type SupportEntry = runtime.SupportEntry

type Config struct {
	Name   string
	Args   []string
	IO     IO
	Env    Env
	Logger Logger
}

type App struct {
	mu      sync.Mutex
	runDone bool
	name    string
	args    []string
	io      runtime.IO
	env     runtime.Env
	logger  runtime.Logger

	handlers *runtime.HandlerRegistry
	mws      []runtime.Middleware
	custom   *customRegistry
}

func New(cfg Config) *App {
	args := cfg.Args
	if len(args) == 0 {
		args = os.Args
	}
	io := cfg.IO
	if io == nil {
		io = process.IO{}
	}
	env := cfg.Env
	if env == nil {
		env = process.Env{}
	}
	logger := cfg.Logger
	if logger == nil {
		logger = runtime.NopLogger{}
	}
	return &App{
		name:     cfg.Name,
		args:     append([]string(nil), args...),
		io:       io,
		env:      env,
		logger:   logger,
		handlers: runtime.NewHandlerRegistry(),
		custom:   newCustomRegistry(),
	}
}

func Supported() []SupportEntry {
	entries := gen.AllSupportEntries()
	out := make([]SupportEntry, len(entries))
	copy(out, entries)
	return out
}

func (a *App) Use(mw Middleware) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.runDone {
		panic("plugin-kit-ai: Use after Run")
	}
	if mw == nil {
		return
	}
	a.mws = append(a.mws, mw)
}

func (a *App) Claude() *claude.Registrar {
	return claude.NewRegistrar(registrarBackend{app: a})
}

func (a *App) Codex() *codex.Registrar {
	return codex.NewRegistrar(registrarBackend{app: a})
}

func (a *App) Run() int {
	return a.RunContext(context.Background())
}

func (a *App) RunContext(ctx context.Context) int {
	a.mu.Lock()
	args := append([]string(nil), a.args...)
	io := a.io
	env := a.env
	logger := a.logger
	mws := append([]runtime.Middleware(nil), a.mws...)
	handlers := a.handlers
	a.runDone = true
	a.mu.Unlock()

	engine := runtime.Engine{
		Args:          args,
		IO:            io,
		Env:           env,
		Logger:        logger,
		Resolver:      a.resolveInvocation,
		Lookup:        a.lookupDescriptor,
		BuildEnvelope: process.BuildEnvelope,
		Handlers:      handlers,
		Middleware:    append([]runtime.Middleware{runtime.RecoveryMiddleware(logger)}, mws...),
	}
	res := engine.Dispatch(ctx)
	if res.Stderr != "" {
		_ = io.WriteStderr(res.Stderr)
	}
	if res.ExitCode == 0 && len(res.Stdout) > 0 {
		if err := io.WriteStdout(res.Stdout); err != nil {
			_ = io.WriteStderr(err.Error() + "\n")
			return 1
		}
	}
	return res.ExitCode
}

type registrarBackend struct {
	app *App
}

func (b registrarBackend) Register(platform runtime.PlatformID, event runtime.EventID, handler runtime.TypedHandler) {
	b.app.mu.Lock()
	defer b.app.mu.Unlock()
	if b.app.runDone {
		panic("plugin-kit-ai: register after Run")
	}
	b.app.handlers.Register(platform, event, handler)
}

func (b registrarBackend) RegisterCustom(rawName string, desc runtime.Descriptor, handler runtime.TypedHandler) error {
	b.app.mu.Lock()
	defer b.app.mu.Unlock()
	if b.app.runDone {
		panic("plugin-kit-ai: register after Run")
	}
	if err := b.app.custom.Register(rawName, desc); err != nil {
		return err
	}
	b.app.handlers.Register(desc.Platform, desc.Event, handler)
	return nil
}

type customRegistry struct {
	byRaw  map[string]runtime.Invocation
	byDesc map[customKey]runtime.Descriptor
}

type customKey struct {
	platform runtime.PlatformID
	event    runtime.EventID
}

func newCustomRegistry() *customRegistry {
	return &customRegistry{
		byRaw:  make(map[string]runtime.Invocation),
		byDesc: make(map[customKey]runtime.Descriptor),
	}
}

func (r *customRegistry) Register(rawName string, desc runtime.Descriptor) error {
	name := strings.TrimSpace(rawName)
	if name == "" {
		return fmt.Errorf("custom hook name required")
	}
	if desc.Platform == "" || desc.Event == "" {
		return fmt.Errorf("custom hook descriptor requires platform and event")
	}
	if desc.Decode == nil || desc.Encode == nil {
		return fmt.Errorf("custom hook descriptor requires decode and encode")
	}
	if _, ok := gen.Lookup(desc.Platform, desc.Event); ok {
		return fmt.Errorf("custom hook %s/%s conflicts with built-in descriptor", desc.Platform, desc.Event)
	}
	if _, err := gen.ResolveInvocation([]string{"plugin-kit-ai", name}, nil); err == nil {
		return fmt.Errorf("custom hook name %q conflicts with built-in invocation", name)
	}
	rawKey := strings.ToLower(name)
	if inv, ok := r.byRaw[rawKey]; ok {
		return fmt.Errorf("custom hook name %q already registered for %s/%s", name, inv.Platform, inv.Event)
	}
	key := customKey{platform: desc.Platform, event: desc.Event}
	if _, ok := r.byDesc[key]; ok {
		return fmt.Errorf("custom hook descriptor already registered for %s/%s", desc.Platform, desc.Event)
	}
	r.byRaw[rawKey] = runtime.Invocation{Platform: desc.Platform, Event: desc.Event, RawName: name}
	r.byDesc[key] = desc
	return nil
}

func (a *App) resolveInvocation(args []string, env runtime.Env) (runtime.Invocation, error) {
	if inv, err := gen.ResolveInvocation(args, env); err == nil {
		return inv, nil
	}
	if len(args) < 2 {
		return runtime.Invocation{}, fmt.Errorf("usage: <binary> <hookName>")
	}
	raw := args[1]
	a.mu.Lock()
	defer a.mu.Unlock()
	if inv, ok := a.custom.byRaw[strings.ToLower(raw)]; ok {
		inv.RawName = raw
		return inv, nil
	}
	return runtime.Invocation{}, fmt.Errorf("unknown invocation %q", raw)
}

func (a *App) lookupDescriptor(platform runtime.PlatformID, event runtime.EventID) (runtime.Descriptor, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if desc, ok := a.custom.byDesc[customKey{platform: platform, event: event}]; ok {
		return desc, true
	}
	return gen.Lookup(platform, event)
}
