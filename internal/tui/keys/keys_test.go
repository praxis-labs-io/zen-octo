package keys_test

import (
	"reflect"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"

	"github.com/zen-octo/zen-octo/internal/tui/keys"
)

// declared pulls every key.Binding off a keymap struct, named by its field, so
// a binding added to a struct is automatically in scope for these tests.
func declared(km any) map[string]key.Binding {
	out := map[string]key.Binding{}
	v := reflect.ValueOf(km)
	t := v.Type()
	for i := range t.NumField() {
		if b, ok := v.Field(i).Interface().(key.Binding); ok {
			out[t.Field(i).Name] = b
		}
	}
	return out
}

// id names a binding by what it is bound to. key.Binding holds a slice, so it
// cannot be compared directly.
func id(b key.Binding) string { return strings.Join(b.Keys(), ",") }

func TestEveryBindingCarriesHelpAndKeys(t *testing.T) {
	tests := []struct {
		name string
		km   any
	}{
		{name: "Global", km: keys.Global},
		{name: "List", km: keys.List},
		{name: "Detail", km: keys.Detail},
		{name: "Form", km: keys.Form},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for field, b := range declared(tt.km) {
				if len(b.Keys()) == 0 {
					t.Errorf("%s is bound to nothing", field)
				}
				if h := b.Help(); h.Key == "" || h.Desc == "" {
					t.Errorf("%s help = %+v, want both a key and a description", field, h)
				}
			}
		})
	}
}

// The form keys are their own context rather than part of the detail screen's.
// tab is deliberately both the tab strip's and the compose box's, and the two
// are never live together: a box takes every key until it closes, which is also
// why Global is not beside it here.
func TestNoKeyIsBoundTwiceInOneContext(t *testing.T) {
	tests := []struct {
		name string
		live []any
	}{
		{name: "list", live: []any{keys.List, keys.Global}},
		{name: "detail", live: []any{keys.Detail, keys.Global}},
		{name: "form", live: []any{keys.Form}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner := map[string]string{}
			for _, km := range tt.live {
				for field, b := range declared(km) {
					for _, k := range b.Keys() {
						if prev, taken := owner[k]; taken {
							t.Errorf("key %q is bound to both %s and %s", k, prev, field)
							continue
						}
						owner[k] = field
					}
				}
			}
		})
	}
}

// The help view is only trustworthy if it stays in step with the declarations
// both ways: nothing declared goes unlisted, and nothing listed is invented.
func TestHelpAndDeclarationsAgree(t *testing.T) {
	tests := []struct {
		name  string
		live  []any
		short []key.Binding
		full  [][]key.Binding
	}{
		{name: "list", live: []any{keys.List, keys.Global}, short: keys.List.ShortHelp(), full: keys.List.FullHelp()},
		// The form keys reach the overlay through the detail screen's help: they
		// are the answer to what tab does inside a box, and the reader asking
		// has only the one overlay to ask.
		{name: "detail", live: []any{keys.Detail, keys.Global, keys.Form}, short: keys.Detail.ShortHelp(keys.DetailContext{Blocks: true, Expand: true, Rail: true}), full: keys.Detail.FullHelp()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isDeclared := map[string]bool{}
			for _, km := range tt.live {
				for _, b := range declared(km) {
					isDeclared[id(b)] = true
				}
			}

			inFullHelp := map[string]bool{}
			for _, group := range tt.full {
				for _, b := range group {
					inFullHelp[id(b)] = true
					if !isDeclared[id(b)] {
						t.Errorf("FullHelp lists %q, which no keymap declares", id(b))
					}
				}
			}
			for _, b := range tt.short {
				if !isDeclared[id(b)] {
					t.Errorf("ShortHelp lists %q, which no keymap declares", id(b))
				}
			}

			for _, km := range tt.live {
				for field, b := range declared(km) {
					if !inFullHelp[id(b)] {
						t.Errorf("%s is declared but never shown in FullHelp", field)
					}
				}
			}
		})
	}
}

// Two rows reading "quit" tell the reader nothing about why there are two, or
// which one works out of a pane that is taking text.
func TestNoTwoBindingsInOneContextReadTheSame(t *testing.T) {
	tests := []struct {
		name string
		live []any
	}{
		{name: "list", live: []any{keys.List, keys.Global}},
		{name: "detail", live: []any{keys.Detail, keys.Global}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner := map[string]string{}
			for _, km := range tt.live {
				for field, b := range declared(km) {
					desc := b.Help().Desc
					if prev, taken := owner[desc]; taken {
						t.Errorf("%s and %s both read %q in the help", prev, field, desc)
						continue
					}
					owner[desc] = field
				}
			}
		})
	}
}
