package input

import "testing"

func TestInitReadlineHistoryNilReaderShouldNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("InitReadlineHistory should handle nil reader, got panic: %v", r)
		}
	}()

	InitReadlineHistory(nil, []string{"a", "b"})
}
