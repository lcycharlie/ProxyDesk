package uistate

import (
	"reflect"
	"testing"

	"proxydesk/internal/routing"
)

func TestPortRangeFromTextUsesDefaults(t *testing.T) {
	got := PortRangeFromText(PortRangeText{Start: "bad", End: ""})
	want := routing.PortRange{Start: routing.DefaultPortStart, End: routing.DefaultPortEnd}
	if got != want {
		t.Fatalf("PortRangeFromText = %+v, want %+v", got, want)
	}
}

func TestValidatePortRangeText(t *testing.T) {
	got, err := ValidatePortRangeText(PortRangeText{Start: "10000", End: "10099"})
	if err != nil {
		t.Fatalf("ValidatePortRangeText returned error: %v", err)
	}
	if got.Start != 10000 || got.End != 10099 {
		t.Fatalf("unexpected port range: %+v", got)
	}
}

func TestAvailablePortOptionsExcludesUsedPorts(t *testing.T) {
	got := AvailablePortOptions(PortRangeText{Start: "10000", End: "10003"}, map[int]bool{
		10001: true,
	})
	want := []string{"10000", "10002", "10003"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("AvailablePortOptions = %v, want %v", got, want)
	}
}

func TestPreferredPortSelection(t *testing.T) {
	text, index := PreferredPortSelection([]string{"10000"}, 10003)
	if text != "10003" || index != -1 {
		t.Fatalf("expected kept port text, got text=%q index=%d", text, index)
	}

	text, index = PreferredPortSelection([]string{"10000"}, 0)
	if text != "" || index != 0 {
		t.Fatalf("expected first option index, got text=%q index=%d", text, index)
	}

	text, index = PreferredPortSelection(nil, 0)
	if text != "" || index != -1 {
		t.Fatalf("expected empty selection, got text=%q index=%d", text, index)
	}
}
