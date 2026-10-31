package runview

import (
	"fmt"
	"slices"
	"unicode/utf16"

	"github.com/pipetower/pipetower-cli/internal/api"
)

// gapMarker marks output which went missing, e.g. while the gateway was unreachable for long.
const gapMarker = "\n… output skipped …\n"

// UnknownOffset marks output whose position in the whole output is unknown, it is appended.
const UnknownOffset = -1

// Tracker is the state of a followed run: the latest REST view of the run and the
// live output of its streams. REST is the source of truth for statuses, the
// streams for output while it is produced.
type Tracker struct {
	Run     *api.Run
	streams map[string]*stream
	details map[string]*api.StepDetails
	// stored is the output of the streams as the API stored it (the head of long output).
	stored map[string]string
}

// stream is the live output of one step.
type stream struct {
	buffer LineBuffer
	// received is the length of the output received so far, in UTF-16 code units like the gateway counts.
	received int
}

// NewTracker creates an empty tracker.
func NewTracker() *Tracker {
	return &Tracker{streams: map[string]*stream{}, details: map[string]*api.StepDetails{}, stored: map[string]string{}}
}

// SetRun takes over a fetched run. Step details (output) of earlier fetches are
// kept, because refetches usually come without output.
func (t *Tracker) SetRun(run api.Run) {
	t.Run = &run
	Walk(t.Run, func(v StepVisit) {
		key := v.Key()
		if v.Step.Details != nil {
			t.details[key] = v.Step.Details
		} else if details, ok := t.details[key]; ok {
			v.Step.Details = details
		}
		if id := v.Step.Stream(); id != "" && v.Step.Details != nil {
			t.stored[id] = v.Step.Details.Output
		}
	})
}

// Output adds live output of a stream at its offset in the whole output of the stream. Live
// chunks and replays (the latest output for late subscribers) overlap: output which was received
// already is skipped, output which went missing in between is marked as a gap.
func (t *Tracker) Output(id, data string, offset int) {
	s := t.stream(id)
	if s.received == 0 && offset > 0 {
		// the live output starts after the gateway's window: the stored output is the head
		s.buffer.Write(t.stored[id] + gapMarker)
		s.received = offset
	}
	s.buffer.Write(s.unseen(data, offset))
}

// unseen returns the part of the data which was not received yet and moves the received length.
func (s *stream) unseen(data string, offset int) string {
	length := utf16Len(data)
	switch {
	case offset == UnknownOffset:
		s.received += length
		return data
	case offset > s.received:
		s.received = offset + length
		return gapMarker + data
	default:
		unseen := dropUTF16(data, s.received-offset)
		s.received = max(s.received, offset+length)
		return unseen
	}
}

// stream returns the stream with the id, creating it on first use.
func (t *Tracker) stream(id string) *stream {
	s, ok := t.streams[id]
	if !ok {
		s = &stream{}
		t.streams[id] = s
	}
	return s
}

// Buffer returns the live output of a step, or nil if none was received.
func (t *Tracker) Buffer(step *api.Step) *LineBuffer {
	s, ok := t.streams[step.Stream()]
	if !ok || s.buffer.Empty() {
		return nil
	}
	return &s.buffer
}

// Lines returns the output lines of a step: live output if any, otherwise the stored output.
func (t *Tracker) Lines(step *api.Step) []string {
	if buffer := t.Buffer(step); buffer != nil {
		return buffer.Lines()
	}
	if step.Details != nil && step.Details.Output != "" {
		var b LineBuffer
		b.Write(step.Details.Output)
		return b.Lines()
	}
	return nil
}

// StepVisit is a step reached by Walk, with its execution and position.
type StepVisit struct {
	Execution *api.Execution
	Step      *api.Step
	Parent    *api.Step
	Depth     int
	Path      []int
}

// Key identifies the step within the run.
func (v StepVisit) Key() string {
	return fmt.Sprintf("%s/%v", v.Execution.ID, v.Path)
}

// Walk visits every step of every execution in order, workflow steps before their action steps.
func Walk(run *api.Run, visit func(StepVisit)) {
	Traverse(run, visit, nil)
}

// Traverse visits every step twice: enter before its children and leave after them (leave may be nil).
func Traverse(run *api.Run, enter, leave func(StepVisit)) {
	for e := range run.Executions {
		execution := &run.Executions[e]
		traverseSteps(execution, nil, execution.Steps, nil, enter, leave)
	}
}

// traverseSteps visits the steps and their children.
func traverseSteps(execution *api.Execution, parent *api.Step, steps []api.Step, path []int, enter, leave func(StepVisit)) {
	for i := range steps {
		step := &steps[i]
		stepPath := slices.Concat(path, []int{i})
		visit := StepVisit{Execution: execution, Step: step, Parent: parent, Depth: len(path), Path: stepPath}

		enter(visit)
		traverseSteps(execution, step, step.Steps, stepPath, enter, leave)
		if leave != nil {
			leave(visit)
		}
	}
}

// utf16Len is the length of the text in UTF-16 code units, the unit of the gateway's offsets.
func utf16Len(text string) int {
	length := 0
	for _, r := range text {
		length += max(1, utf16.RuneLen(r))
	}
	return length
}

// dropUTF16 removes the first n UTF-16 code units of the text (a split character is dropped whole).
func dropUTF16(text string, n int) string {
	for i, r := range text {
		if n <= 0 {
			return text[i:]
		}
		n -= max(1, utf16.RuneLen(r))
	}
	return ""
}
