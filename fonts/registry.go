package fonts

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/aaripurna/sanur-pdf/core"
)

// Registry maps names to fonts.
//
// Its reason to exist is configuration. A core.Font is a parsed set of glyph
// metrics, so it cannot come out of a JSON file or a command-line flag — but a name
// can, and a registry is what turns one back into the other. Layout code that
// builds styles in Go has no need for it.
//
// A Registry is safe for concurrent use. Fonts are typically registered once at
// startup and read from many goroutines afterwards.
type Registry struct {
	mu    sync.RWMutex
	fonts map[string]core.Font
}

// NewRegistry creates a registry holding the built-in faces.
//
// The standard-14 are always present, so a configuration naming "Helvetica-Bold"
// works with no setup at all — which is the point of shipping their metrics.
func NewRegistry() *Registry {
	r := &Registry{fonts: make(map[string]core.Font, len(standard14))}
	for name, font := range standard14 {
		r.fonts[name] = font
	}
	return r
}

// defaultRegistry backs the package-level functions.
var defaultRegistry = NewRegistry()

// Default returns the shared registry.
//
// Most programs want exactly one, populated at startup. Code needing isolation —
// tests, or a server rendering with per-tenant fonts — should build its own with
// NewRegistry rather than reaching for this.
func Default() *Registry { return defaultRegistry }

// Register adds a font under a name, replacing any previous entry.
//
// Replacing rather than refusing is deliberate: overriding a built-in face with a
// real Helvetica, or swapping a placeholder for the licensed font once it arrives,
// is a reasonable thing to want. A collision here is a choice, unlike the
// accidental collisions that destination names guard against.
func (r *Registry) Register(name string, font core.Font) error {
	if name == "" {
		return fmt.Errorf("sanur/fonts: font name must not be empty")
	}
	if font == nil {
		return fmt.Errorf("sanur/fonts: cannot register a nil font as %q", name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.fonts[name] = font
	return nil
}

// Lookup finds a font by name.
func (r *Registry) Lookup(name string) (core.Font, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	font, ok := r.fonts[name]
	return font, ok
}

// Resolve finds a font by name, reporting what is available when it is missing.
//
// The available names go in the error because a bad font name is nearly always a
// typo or a missing registration, and both are far quicker to fix when the message
// says what the alternatives were.
func (r *Registry) Resolve(name string) (core.Font, error) {
	if font, ok := r.Lookup(name); ok {
		return font, nil
	}
	return nil, fmt.Errorf(
		"sanur/fonts: no font registered as %q (available: %s)",
		name, strings.Join(r.Names(), ", "))
}

// Names lists the registered names in a stable order.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.fonts))
	for name := range r.fonts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// LoadTrueType parses a font file and registers it.
//
// An empty name derives one from the file, matching LoadTrueTypeFile.
func (r *Registry) LoadTrueType(name, path string) (core.Font, error) {
	font, err := LoadTrueTypeFile(name, path)
	if err != nil {
		return nil, err
	}
	if err := r.Register(font.Name(), font); err != nil {
		return nil, err
	}
	return font, nil
}

// RegisterTrueTypeBytes parses a font from memory and registers it.
func (r *Registry) RegisterTrueTypeBytes(name string, data []byte) (core.Font, error) {
	font, err := RegisterTrueType(name, data)
	if err != nil {
		return nil, err
	}
	if err := r.Register(name, font); err != nil {
		return nil, err
	}
	return font, nil
}

// Register adds a font to the shared registry.
func Register(name string, font core.Font) error {
	return defaultRegistry.Register(name, font)
}

// Lookup finds a font in the shared registry.
func Lookup(name string) (core.Font, bool) { return defaultRegistry.Lookup(name) }

// Resolve finds a font in the shared registry, or explains what is available.
func Resolve(name string) (core.Font, error) { return defaultRegistry.Resolve(name) }

// RegisteredNames lists the names in the shared registry.
func RegisteredNames() []string { return defaultRegistry.Names() }
