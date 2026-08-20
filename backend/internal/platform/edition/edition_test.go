package edition

import (
	"reflect"
	"testing"
)

func TestParseEditions(t *testing.T) {
	tests := []struct {
		input     string
		wantNames []string
	}{
		{"", []string{"agent", "core", "enterprise", "saasops"}},                     // empty → all
		{"   ", []string{"agent", "core", "enterprise", "saasops"}},                  // blank → all
		{"core", []string{"core"}},
		{"CORE", []string{"core"}},                                                   // case insensitive
		{"core,enterprise", []string{"core", "enterprise"}},
		{"core,enterprise,agent,saasops", []string{"agent", "core", "enterprise", "saasops"}},
		{"core, enterprise", []string{"core", "enterprise"}},                         // spaces trimmed
		{"unknown", []string{"agent", "core", "enterprise", "saasops"}},              // all unknowns → all
		{"core,bogus", []string{"core"}},                                             // unknown token skipped
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			g := ParseEditions(tt.input)
			got := g.Names()
			if !reflect.DeepEqual(got, tt.wantNames) {
				t.Errorf("ParseEditions(%q).Names() = %v, want %v", tt.input, got, tt.wantNames)
			}
		})
	}
}

func TestGate_Has(t *testing.T) {
	all := AllEditions()
	if !all.Has(Core)            { t.Error("AllEditions should have Core") }
	if !all.Has(Enterprise)      { t.Error("AllEditions should have Enterprise") }
	if !all.Has(AgentAutomation) { t.Error("AllEditions should have AgentAutomation") }
	if !all.Has(SaaSOpsAddOn)    { t.Error("AllEditions should have SaaSOpsAddOn") }

	coreOnly := NewGate(Core)
	if !coreOnly.Has(Core)            { t.Error("coreOnly should have Core") }
	if coreOnly.Has(Enterprise)       { t.Error("coreOnly should not have Enterprise") }
	if coreOnly.Has(AgentAutomation)  { t.Error("coreOnly should not have AgentAutomation") }
	if coreOnly.Has(SaaSOpsAddOn)     { t.Error("coreOnly should not have SaaSOpsAddOn") }
}

func TestGate_Names_Order(t *testing.T) {
	// Names must always return sorted output regardless of bit order.
	g := NewGate(SaaSOpsAddOn | Core | Enterprise | AgentAutomation)
	got := g.Names()
	want := []string{"agent", "core", "enterprise", "saasops"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Names() = %v, want %v (must be sorted)", got, want)
	}
}
