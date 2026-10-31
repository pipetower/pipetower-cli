package runview

import (
	"reflect"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestCarriageReturnOverwritesLine(t *testing.T) {
	var b LineBuffer
	b.Write("Counting:  1% (1/100)\rCounting: 50% (50/100)\rCounting: 100% (100/100), done.\n")
	b.Write("next")

	want := []string{"Counting: 100% (100/100), done.", "next"}
	if got := b.Tail(5); !reflect.DeepEqual(got, want) {
		t.Errorf("Tail = %q, want %q", got, want)
	}
}

func TestCRLFKeepsLine(t *testing.T) {
	var b LineBuffer
	b.Write("one\r\ntwo\r\n")

	if got := b.Flush(); !reflect.DeepEqual(got, []string{"one", "two"}) {
		t.Errorf("Flush = %q", got)
	}
}

func TestColorsKeptOtherSequencesDropped(t *testing.T) {
	var b LineBuffer
	b.Write("\x1b[32m✓ built\x1b[0m\x1b[2K\x1b]0;title\x07 done\n")

	if got := b.Flush(); !reflect.DeepEqual(got, []string{"\x1b[32m✓ built\x1b[0m done"}) {
		t.Errorf("Flush = %q", got)
	}
}

func TestEscapeSequenceSplitAcrossWrites(t *testing.T) {
	var b LineBuffer
	b.Write("a\x1b[3")
	b.Write("1mred\x1b")
	b.Write("[0m\n")

	if got := b.Flush(); !reflect.DeepEqual(got, []string{"a\x1b[31mred\x1b[0m"}) {
		t.Errorf("Flush = %q", got)
	}
}

func TestFlushReturnsOnlyNewLines(t *testing.T) {
	var b LineBuffer
	b.Write("1\n2\n")
	b.Flush()
	b.Write("3\npartial")

	if got := b.Flush(); !reflect.DeepEqual(got, []string{"3"}) {
		t.Errorf("Flush = %q", got)
	}
	if got := b.FlushAll(); !reflect.DeepEqual(got, []string{"partial"}) {
		t.Errorf("FlushAll = %q", got)
	}
}

func TestBufferIsCapped(t *testing.T) {
	var b LineBuffer
	for i := 0; i < maxLines+10; i++ {
		b.Write("x\n")
	}

	if len(b.lines) != maxLines || b.dropped != 10 {
		t.Errorf("lines = %d, dropped = %d", len(b.lines), b.dropped)
	}
	if got := len(b.Flush()); got != maxLines {
		t.Errorf("Flush = %d lines", got)
	}
}

func TestEscapeSequencesLeaveNoText(t *testing.T) {
	var b LineBuffer
	b.Write("\x1b[1mbold\x1b(B\x1b[m plain\x1bPdcs payload\x1b\\ \x1b_apc\x07end\n")

	if got := ansi.Strip(b.Lines()[0]); got != "bold plain end" {
		t.Errorf("line = %q", got)
	}
}

func TestColorsAfterCarriageReturnAreKept(t *testing.T) {
	var b LineBuffer
	b.Write("50%\r\x1b[32m100%\x1b[0m\n")

	if got := b.Lines()[0]; got != "\x1b[32m100%\x1b[0m" {
		t.Errorf("line = %q", got)
	}
}
