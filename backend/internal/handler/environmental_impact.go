package handler

import "strings"

// Factor provenance/version:
//   - Carbon factors are deterministic category-level estimates adapted from
//     Poore & Nemecek, Science 360 (2018), the global food-supply-chain dataset
//     published with "Reducing food's environmental impacts through producers
//     and consumers". Values are kg CO2e per kg of food.
//   - Virtual-water factors use Water Footprint Network global-average product
//     footprints (waterfootprint.org), litres per kg. The explicitly published
//     livestock averages are beef 15,400; sheep 10,400; pork 6,000; chicken
//     4,300; eggs 3,300; and milk 1,000 L/kg.
//   - EPA Greenhouse Gas Equivalencies Calculator (2024 methodology) estimates
//     0.060 metric ton CO2 per urban tree planted per year, averaged across its
//     first ten years of growth. One tree-year equivalent is therefore 60 kg.
//
// These estimates are suitable for product feedback, not lifecycle assessment.
const (
	environmentalFactorVersion = "poore-nemecek-2018+wfn-global-average+epa-2024"
	epaTreeYearKGCO2e          = 60.0
)

type environmentalFactor struct {
	kgCO2ePerKG        float64
	virtualWaterLPerKG float64
}

type wasteImpact struct {
	KGCO2e          float64
	VirtualWaterL   float64
	TreeEquivalents float64
}

var environmentalFactors = map[string]environmentalFactor{
	"beef":       {kgCO2ePerKG: 99.48, virtualWaterLPerKG: 15400},
	"lamb":       {kgCO2ePerKG: 39.72, virtualWaterLPerKG: 10400},
	"pork":       {kgCO2ePerKG: 12.31, virtualWaterLPerKG: 6000},
	"chicken":    {kgCO2ePerKG: 9.87, virtualWaterLPerKG: 4300},
	"eggs":       {kgCO2ePerKG: 4.67, virtualWaterLPerKG: 3300},
	"milk":       {kgCO2ePerKG: 3.15, virtualWaterLPerKG: 1000},
	"cheese":     {kgCO2ePerKG: 23.88, virtualWaterLPerKG: 5000},
	"rice":       {kgCO2ePerKG: 4.45, virtualWaterLPerKG: 2500},
	"grains":     {kgCO2ePerKG: 1.57, virtualWaterLPerKG: 1800},
	"vegetables": {kgCO2ePerKG: 0.53, virtualWaterLPerKG: 322},
	"fruit":      {kgCO2ePerKG: 0.86, virtualWaterLPerKG: 962},
	"meat":       {kgCO2ePerKG: 20.00, virtualWaterLPerKG: 7000},
	"prepared":   {kgCO2ePerKG: 3.00, virtualWaterLPerKG: 1000},
	"other":      {kgCO2ePerKG: 2.50, virtualWaterLPerKG: 1000},
}

func environmentalFactorFor(category, name string) environmentalFactor {
	text := strings.ToLower(strings.TrimSpace(category + " " + name))
	checks := []struct {
		key   string
		terms []string
	}{
		{key: "beef", terms: []string{"beef", "steak", "牛"}},
		{key: "lamb", terms: []string{"lamb", "mutton", "sheep", "羊"}},
		{key: "pork", terms: []string{"pork", "pig", "bacon", "ham", "豚"}},
		{key: "chicken", terms: []string{"chicken", "poultry", "turkey", "鶏"}},
		{key: "eggs", terms: []string{"egg", "卵"}},
		{key: "cheese", terms: []string{"cheese", "チーズ"}},
		{key: "milk", terms: []string{"milk", "dairy", "yogurt", "乳", "ヨーグルト"}},
		{key: "rice", terms: []string{"rice", "米", "ご飯"}},
		{key: "grains", terms: []string{"grain", "bread", "wheat", "pasta", "cereal", "パン", "小麦"}},
		{key: "vegetables", terms: []string{"vegetable", "spinach", "tomato", "potato", "carrot", "野菜"}},
		{key: "fruit", terms: []string{"fruit", "apple", "banana", "berry", "orange", "果物"}},
		{key: "meat", terms: []string{"meat", "肉"}},
		{key: "prepared", terms: []string{"prepared", "meal", "dish", "ready"}},
	}
	for _, check := range checks {
		for _, term := range check.terms {
			if strings.Contains(text, term) {
				return environmentalFactors[check.key]
			}
		}
	}
	return environmentalFactors["other"]
}

func estimateWasteImpact(category, name string, quantityG float64) wasteImpact {
	if quantityG <= 0 {
		return wasteImpact{}
	}
	factor := environmentalFactorFor(category, name)
	quantityKG := quantityG / 1000
	kgCO2e := quantityKG * factor.kgCO2ePerKG
	return wasteImpact{
		KGCO2e:          kgCO2e,
		VirtualWaterL:   quantityKG * factor.virtualWaterLPerKG,
		TreeEquivalents: kgCO2e / epaTreeYearKGCO2e,
	}
}
