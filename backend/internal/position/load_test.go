package position

import (
	"reflect"
	"testing"

	"github.com/AthillaZaidan/datanest/backend/internal/models"
)

func TestEffectiveLeavesOutRunsThatNeverStarted(t *testing.T) {
	const a, b = 1, 2
	versions := map[uint]int{a: 1}
	tests := []struct {
		name    string
		events  []Event
		exclude uint
		want    []uint
	}{
		{"a failed run without a version", []Event{ev(1, a, run, ok), ev(2, b, run, failed)}, 0, []uint{1}},
		{"a failed run with a version", []Event{ev(1, a, run, failed)}, 0, []uint{1}},
		{"only failed runs", []Event{ev(1, b, run, ok), ev(2, b, run, partial), ev(3, b, run, models.ExecUncertain)}, 0, []uint{1, 2, 3}},
		{"only runs", []Event{ev(1, b, forward, failed), undo(2, b, 1, failed)}, 0, []uint{1, 2}},
		{"a failed run that was run again", []Event{ev(1, a, run, failed), ev(2, a, run, failed)}, 0, []uint{2}},
		{"a failed run that was requested again", []Event{ev(1, a, run, failed), ev(2, a, run, models.ExecCanceled)}, 0, []uint{2}},
		{"a failed run listed after the run that replaced it", []Event{ev(2, a, run, ok), ev(1, a, run, failed)}, 0, []uint{2}},
		{"a failed run whose replacement is excluded", []Event{ev(1, a, run, failed), ev(2, a, run, models.ExecRunning)}, 2, []uint{}},
		{"a failed run with only a later forward", []Event{ev(1, a, run, failed), ev(2, a, forward, ok)}, 0, []uint{1, 2}},
		{"the excluded execution", []Event{ev(1, a, run, ok), undo(2, a, 1, ok)}, 2, []uint{1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := []uint{}
			for _, e := range effective(tt.events, versions, tt.exclude) {
				got = append(got, e.ID)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("events = %v, want %v", got, tt.want)
			}
		})
	}
}
