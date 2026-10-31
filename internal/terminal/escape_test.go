package terminal

import (
	"reflect"
	"testing"
)

func TestEscapeAtLineStart(t *testing.T) {
	e := NewEscaper()

	got := e.Feed([]byte("ls\r~."))

	want := []Action{{Input: []byte("ls\r")}, {Command: CommandDetach}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Feed = %+v", got)
	}
}

func TestTildeInsideLineIsInput(t *testing.T) {
	e := NewEscaper()
	e.Feed([]byte("x"))

	got := e.Feed([]byte("cd ~."))

	if !reflect.DeepEqual(got, []Action{{Input: []byte("cd ~.")}}) {
		t.Errorf("Feed = %+v", got)
	}
}

func TestEscapeSpanningChunksAndLiteralTilde(t *testing.T) {
	e := NewEscaper()

	first := e.Feed([]byte("~"))
	second := e.Feed([]byte("~x\r~?"))

	if len(first) != 0 {
		t.Errorf("first = %+v", first)
	}
	want := []Action{{Input: []byte("~x\r")}, {Command: CommandHelp}}
	if !reflect.DeepEqual(second, want) {
		t.Errorf("second = %+v", second)
	}
}

func TestUnknownEscapeKeyIsSent(t *testing.T) {
	e := NewEscaper()

	got := e.Feed([]byte("~z"))

	if !reflect.DeepEqual(got, []Action{{Input: []byte("~z")}}) {
		t.Errorf("Feed = %+v", got)
	}
}
