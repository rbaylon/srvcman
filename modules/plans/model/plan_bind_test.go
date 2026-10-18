package planmodel

import "testing"

func TestPlanQueueBandwidthMustBeUsableByPf(t *testing.T) {
	ok := Plan{Plan: "Plan799", Downspeed: 50, Upspeed: 30, SpeedTestDown: 100, SpeedTestUp: 100}
	if err := ok.Bind(nil); err != nil {
		t.Fatalf("a good plan was refused: %v", err)
	}
	for name, p := range map[string]Plan{
		"zero download":  {Plan: "A", SpeedTestDown: 0, SpeedTestUp: 50},
		"zero upload":    {Plan: "A", SpeedTestDown: 50, SpeedTestUp: 0},
		"below pf min":   {Plan: "A", SpeedTestDown: 4, SpeedTestUp: 50},
		"negative":       {Plan: "A", SpeedTestDown: 50, SpeedTestUp: -1},
		"absurdly large": {Plan: "A", SpeedTestDown: 50, SpeedTestUp: 1000000},
	} {
		if err := p.Bind(nil); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}
