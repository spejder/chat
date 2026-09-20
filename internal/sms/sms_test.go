package sms_test

import (
	"strings"
	"testing"

	"github.com/spejder/chat/internal/sms"
)

// TestStdoutSenderPrintsTheMessage makes sure that a developer can read the
// number and the text in the terminal.
func TestStdoutSenderPrintsTheMessage(t *testing.T) {
	t.Parallel()

	var out strings.Builder

	sender := sms.StdoutSender{Writer: &out}

	message := sms.Message{To: "+4521650113", Text: "Your code for Chat is 123456."}
	if err := sender.Send(t.Context(), message); err != nil {
		t.Fatalf("send: %v", err)
	}

	printed := out.String()
	for _, want := range []string{message.To, message.Text, "SMS"} {
		if !strings.Contains(printed, want) {
			t.Errorf("the output misses %q:\n%s", want, printed)
		}
	}
}

// TestRecorderKeepsTheMessages covers the sender that the tests use.
func TestRecorderKeepsTheMessages(t *testing.T) {
	t.Parallel()

	recorder := &sms.Recorder{}

	if _, ok := recorder.Last(); ok {
		t.Error("an empty recorder returns a message")
	}

	for _, text := range []string{"first", "second"} {
		if err := recorder.Send(t.Context(), sms.Message{To: "+4521650113", Text: text}); err != nil {
			t.Fatalf("send: %v", err)
		}
	}

	if len(recorder.Messages()) != 2 {
		t.Errorf("the recorder holds %d messages, want 2", len(recorder.Messages()))
	}

	last, ok := recorder.Last()
	if !ok || last.Text != "second" {
		t.Errorf("the last message is %+v, want the second one", last)
	}
}
