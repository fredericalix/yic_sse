package sse

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// AddEvent add SSE a event to the writter but the event will not be sent.
func AddEvent(w io.Writer, event string, data string) error {
	if event != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
			return err
		}
	}
	if len(data) == 0 {
		_, err := w.Write([]byte("data: \n\n"))
		return err
	}

	scanner := bufio.NewScanner(strings.NewReader(data))
	for scanner.Scan() {
		if _, err := fmt.Fprintf(w, "data: %s\n", scanner.Text()); err != nil {
			return err
		}
	}
	_, err := w.Write([]byte("\n"))
	return err
}

// Send the queue event by AddEvent.
func Send(w http.ResponseWriter) {
	w.(http.Flusher).Flush()
}

// SendEvent add a SSE event and Send it. The other pending event will be sent too.
func SendEvent(w http.ResponseWriter, event string, data string) error {
	err := AddEvent(w, event, data)
	if err != nil {
		return err
	}
	Send(w)
	return nil
}
