package lockstep

import "testing"

func TestReplaySerializeDeserialize(t *testing.T) {
	replay := NewReplay()
	replay.SetUnitInfos([][]byte{[]byte("unit")})
	input := NewOneFrameInputs()
	input.Inputs[1] = &LSInput{Button: 2}
	replay.AddFrameInput(input)
	replay.AddSnapshot(10, []byte("snap"))

	data, err := replay.Serialize()
	if err != nil {
		t.Fatalf("Serialize err = %v", err)
	}
	clone := NewReplay()
	if err := clone.Deserialize(data); err != nil {
		t.Fatalf("Deserialize err = %v", err)
	}
	if len(clone.UnitInfos()) != 1 {
		t.Fatalf("unit infos = %d", len(clone.UnitInfos()))
	}
	if len(clone.GetFrameInputsRange(0, 1)) != 1 {
		t.Fatal("frame range missing")
	}
	if len(clone.Snapshots()) != 1 {
		t.Fatal("snapshots missing")
	}
}
