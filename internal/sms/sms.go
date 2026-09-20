// Package sms sends short messages.
//
// The application has no provider yet. StdoutSender prints what a provider
// would have sent, so a developer can read the code in the terminal.
package sms

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

// Message is one short message.
type Message struct {
	// To is a phone number in the international form, for example
	// +4521650113.
	To   string
	Text string
}

// Sender hands a message to a provider.
type Sender interface {
	Send(ctx context.Context, message Message) error
}

// StdoutSender prints the message instead of sending it.
type StdoutSender struct {
	// Writer receives the message. An empty field means standard output.
	Writer io.Writer
}

// Send prints the message between two rules, so it stands out in a log.
func (s StdoutSender) Send(_ context.Context, message Message) error {
	writer := s.Writer
	if writer == nil {
		writer = os.Stdout
	}

	var out strings.Builder

	out.WriteString("\n--- SMS, not sent, no provider yet ---\n")
	fmt.Fprintf(&out, "To: %s\n", message.To)
	out.WriteString(message.Text)
	out.WriteString("\n--------------------------------------\n\n")

	if _, err := io.WriteString(writer, out.String()); err != nil {
		return fmt.Errorf("print the message: %w", err)
	}

	return nil
}

// Recorder keeps the messages in memory. The tests use it.
type Recorder struct {
	mu       sync.Mutex
	messages []Message
}

// Send keeps the message.
func (r *Recorder) Send(_ context.Context, message Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.messages = append(r.messages, message)

	return nil
}

// Messages returns a copy of what the recorder holds.
func (r *Recorder) Messages() []Message {
	r.mu.Lock()
	defer r.mu.Unlock()

	return append([]Message(nil), r.messages...)
}

// Last returns the newest message. The second value is false when the
// recorder is empty.
func (r *Recorder) Last() (Message, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.messages) == 0 {
		return Message{}, false
	}

	return r.messages[len(r.messages)-1], true
}
