package sse

import (
	"testing"

	"bytes"
)

func TestSSEEmpty(t *testing.T) {
	wanted := "data: \n\n"
	w := bytes.NewBuffer(make([]byte, 0, 1024))

	err := AddEvent(w, "", "")
	if err != nil {
		t.Error(err)
	}
	got := w.String()
	if got != wanted {
		t.Errorf("empty event and data, got: %v wanted: %v", got, wanted)
	}
}

func TestSSEOnLine(t *testing.T) {
	wanted := "data: test on only one line\n\n"
	w := bytes.NewBuffer(make([]byte, 0, 1024))

	err := AddEvent(w, "", "test on only one line")
	if err != nil {
		t.Error(err)
	}
	got := w.String()
	if got != wanted {
		t.Errorf("got: %v wanted: %v", got, wanted)
	}
}

func TestSSEOnLineEvent(t *testing.T) {
	wanted := "event: super event\ndata: test on only one line\n\n"
	w := bytes.NewBuffer(make([]byte, 0, 1024))

	err := AddEvent(w, "super event", "test on only one line")
	if err != nil {
		t.Error(err)
	}
	got := w.String()
	if got != wanted {
		t.Errorf("got: %v wanted: %v", got, wanted)
	}
}

func TestSSEMultipleLine(t *testing.T) {
	wanted := "data: test on multiple lines\ndata: 1 line\ndata: 2 lines\ndata: the last one.\n\n"
	w := bytes.NewBuffer(make([]byte, 0, 1024))

	err := AddEvent(w, "", "test on multiple lines\n1 line\n2 lines\nthe last one.")
	if err != nil {
		t.Error(err)
	}
	got := w.String()
	if got != wanted {
		t.Errorf("got: %v wanted: %v", got, wanted)
	}
}

func TestSSEMultipleLineEvent(t *testing.T) {
	wanted := "event: mega event\ndata: test on multiple lines\ndata: 1 line\ndata: 2 lines\ndata: the last one.\n\n"
	w := bytes.NewBuffer(make([]byte, 0, 1024))

	err := AddEvent(w, "mega event", "test on multiple lines\n1 line\n2 lines\nthe last one.")
	if err != nil {
		t.Error(err)
	}
	got := w.String()
	if got != wanted {
		t.Errorf("got: %v wanted: %v", got, wanted)
	}
}
