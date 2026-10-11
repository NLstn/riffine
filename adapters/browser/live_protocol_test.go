package browser

import (
	"math"
	"testing"
)

func TestLiveAcknowledgementsPreserveIdentifiers(t *testing.T) {
	a, err := decodeLiveAcks(`[{"generation":"18446744073709551615","sequence":"9007199254740993","consumedOutputFrame":9007199254740991}]`, 128)
	if err != nil || len(a) != 1 || a[0].Generation != math.MaxUint64 || a[0].Sequence != 9007199254740993 || a[0].ConsumedOutputFrame != 9007199254740991 {
		t.Fatalf("acknowledgements=%+v, error=%v", a, err)
	}
}
func TestLiveAcknowledgementsRejectInvalidEvidence(t *testing.T) {
	for _, text := range []string{
		`[{"generation":"01","sequence":"1","consumedOutputFrame":1}]`,
		`[{"generation":"1","sequence":"0","consumedOutputFrame":1}]`,
		`[{"generation":"18446744073709551616","sequence":"1","consumedOutputFrame":1}]`,
		`[{"generation":"1","sequence":"1","consumedOutputFrame":9007199254740992}]`,
		`[{"generation":"1","sequence":"1","consumedOutputFrame":-1}]`,
		`[{"generation":"1","sequence":"1","consumedOutputFrame":1,"copied":true}]`,
		`[] {}`, `[{"generation":"1","sequence":"1"}]`, `[{"generation":"1","sequence":"1","consumedOutputFrame":1},{"generation":"1","sequence":"2","consumedOutputFrame":2}]`,
	} {
		if _, err := decodeLiveAcks(text, 1); err == nil {
			t.Fatalf("accepted %s", text)
		}
	}
}
