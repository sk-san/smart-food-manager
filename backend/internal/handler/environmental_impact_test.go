package handler

import (
	"math"
	"testing"
)

func TestEstimateWasteImpactUsesSourcedUnits(t *testing.T) {
	impact := estimateWasteImpact("beef", "Steak", 1000)
	if impact.KGCO2e != 99.48 {
		t.Errorf("CO2e = %v, want 99.48 kg", impact.KGCO2e)
	}
	if impact.VirtualWaterL != 15400 {
		t.Errorf("water = %v, want 15400 L", impact.VirtualWaterL)
	}
	if want := 99.48 / epaTreeYearKGCO2e; math.Abs(impact.TreeEquivalents-want) > 1e-12 {
		t.Errorf("trees = %v, want %v", impact.TreeEquivalents, want)
	}
	if environmentalFactorVersion == "" {
		t.Error("environmental factor version must be declared")
	}
}

func TestEstimateWasteImpactScalesAndMatchesNames(t *testing.T) {
	impact := estimateWasteImpact("", "Chicken breast", 250)
	if math.Abs(impact.KGCO2e-2.4675) > 1e-12 || impact.VirtualWaterL != 1075 {
		t.Errorf("chicken impact = %+v", impact)
	}
	if got := estimateWasteImpact("vegetable", "Spinach", 0); got != (wasteImpact{}) {
		t.Errorf("zero quantity impact = %+v", got)
	}
}
