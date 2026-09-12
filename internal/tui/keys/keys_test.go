package keys_test

import (
	"reflect"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"

	"github.com/praxis-labs-io/zen-octo/internal/tui/keys"
)

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

func id(b key.Binding) string { return strings.Join(b.Keys(), ",") }

func label(b key.Binding) string { return id(b) + " " + b.Help().Desc }

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

func TestHelpAndDeclarationsAgree(t *testing.T) {
	tests := []struct {
		name  string
		live  []any
		short []key.Binding
		full  [][]key.Binding
	}{
		{name: "list", live: []any{keys.List, keys.Global}, short: keys.List.ShortHelp(keys.ListContext{Rows: true, Search: true}), full: keys.List.FullHelp()},
		{name: "list blocked", live: []any{keys.List, keys.Global}, short: keys.List.ShortHelp(keys.ListContext{}), full: keys.List.FullHelp()},
		{name: "list search", live: []any{keys.List, keys.Global}, short: keys.List.SearchHelp(), full: keys.List.FullHelp()},
		{name: "detail", live: []any{keys.Detail, keys.Global, keys.Form}, short: keys.Detail.ShortHelp(keys.DetailContext{Blocks: true, Expand: true, Rail: true, Column: "file"}), full: keys.Detail.FullHelp()},
		{name: "detail rail", live: []any{keys.Detail, keys.Global, keys.Form}, short: keys.Detail.ShortHelp(keys.DetailContext{Activate: true, Panes: true, Rail: true}), full: keys.Detail.FullHelp()},
		{name: "detail searched", live: []any{keys.Detail, keys.Global, keys.Form}, short: keys.Detail.ShortHelp(keys.DetailContext{SearchStanding: true, JobLog: true, JobMatches: true}), full: keys.Detail.FullHelp()},
		{name: "detail search", live: []any{keys.Detail, keys.Global, keys.Form}, short: keys.Detail.SearchHelp(), full: keys.Detail.FullHelp()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isDeclared, isLabelled := map[string]bool{}, map[string]bool{}
			for _, km := range tt.live {
				for _, b := range declared(km) {
					isDeclared[id(b)] = true
					isLabelled[label(b)] = true
				}
			}

			inFullHelp := map[string]bool{}
			for _, group := range tt.full {
				for _, b := range group {
					inFullHelp[label(b)] = true
					if !isLabelled[label(b)] {
						t.Errorf("FullHelp lists %q, which no keymap declares", label(b))
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
					if !inFullHelp[label(b)] {
						t.Errorf("%s is declared but never shown in FullHelp", field)
					}
				}
			}
		})
	}
}

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
