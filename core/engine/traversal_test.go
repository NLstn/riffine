package engine

import "testing"

func TestTraversalConsumptionGate(t *testing.T) {
	schedule := Schedule{Generation: 2, Sequence: 4, Source: "source", StartFrame: 12000, EndFrame: 36000, OutputStart: 100, OutputEnd: 32100, Speed: .75, Pitch: 3}
	gate, err := NewTraversal(schedule)
	if err != nil {
		t.Fatal(err)
	}
	for _, ack := range []OutputAcknowledgement{{1, 4, 32100}, {2, 3, 32100}, {2, 4, 20000}} {
		event, err := gate.Acknowledge(ack)
		if err != nil || event != nil {
			t.Fatalf("premature completion: %+v %v", event, err)
		}
	}
	if _, err := gate.Acknowledge(OutputAcknowledgement{2, 4, 15000}); err == nil {
		t.Fatal("accepted regressing consumption")
	}
	if _, err := gate.Acknowledge(OutputAcknowledgement{2, 4, 32101}); err == nil {
		t.Fatal("accepted consumption beyond schedule")
	}
	event, err := gate.Acknowledge(OutputAcknowledgement{2, 4, 32100})
	if err != nil || event == nil || event.StartFrame != 12000 || event.EndFrame != 36000 || event.Type != "traversal_completed" {
		t.Fatalf("completion: %+v %v", event, err)
	}
	event, err = gate.Acknowledge(OutputAcknowledgement{2, 4, 32100})
	if err != nil || event != nil {
		t.Fatal("duplicate completion")
	}
}
func TestTraversalRejectsInvalidSchedule(t *testing.T) {
	if _, err := NewTraversal(Schedule{}); err == nil {
		t.Fatal("accepted empty schedule")
	}
}
