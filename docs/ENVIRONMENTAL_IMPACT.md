# Environmental-impact estimates

Waste events expose three estimates derived from the discarded edible mass:

```text
kg CO2e       = discarded grams / 1,000 × category kg CO2e/kg
virtual water = discarded grams / 1,000 × category litres/kg
tree-years    = kg CO2e / 60
```

The backend chooses a deterministic category factor from the persisted scanned
category and food name, then falls back to a generic `other` factor. Results
are reproducible within the named factor version and do not require the AI
provider when waste is listed.

| Matched category | kg CO2e/kg | Virtual water L/kg |
| --- | ---: | ---: |
| Beef | 99.48 | 15,400 |
| Lamb | 39.72 | 10,400 |
| Pork | 12.31 | 6,000 |
| Chicken/poultry | 9.87 | 4,300 |
| Eggs | 4.67 | 3,300 |
| Milk/dairy | 3.15 | 1,000 |
| Cheese | 23.88 | 5,000 |
| Rice | 4.45 | 2,500 |
| Grains | 1.57 | 1,800 |
| Vegetables | 0.53 | 322 |
| Fruit | 0.86 | 962 |
| Generic meat | 20.00 | 7,000 |
| Prepared food | 3.00 | 1,000 |
| Other fallback | 2.50 | 1,000 |

## Sources and interpretation

- Greenhouse-gas factors are category-level global estimates adapted from
  Poore & Nemecek (2018), *Reducing food's environmental impacts through
  producers and consumers*, as published in the Our World in Data food-impact
  dataset: <https://ourworldindata.org/grapher/ghg-per-kg-poore>.
- Virtual-water factors use Water Footprint Network global product averages:
  <https://www.waterfootprint.org/resources/interactive-tools/product-gallery/>.
- The tree conversion uses the US EPA estimate of 0.060 metric ton CO2 per
  urban tree planted per year, averaged over ten years of growth:
  <https://www.epa.gov/energy/greenhouse-gas-equivalencies-calculator-calculations-and-references>.

`tree_equivalents` therefore means **urban-tree-year equivalents**, not trees
that the application has planted. Water values combine global-average green,
blue, and grey water footprints. All three outputs are directional product
feedback, not site-specific lifecycle-assessment or emissions-inventory data.

The factor version used by the backend is
`poore-nemecek-2018+wfn-global-average+epa-2024`; the executable table lives in
`backend/internal/handler/environmental_impact.go`. Updating that table changes
recomputed historical estimates, because impacts are calculated at read time
rather than persisted on each waste event. Every API waste response includes
the active version as `impact_factor_version`.
