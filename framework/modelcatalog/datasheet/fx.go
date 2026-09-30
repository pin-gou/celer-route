package datasheet

// FX layer for D11-A. Under the chosen USD-everywhere strategy the price book
// is single-currency: every per-token rate on the TableModelPricing row is
// stored in USD, and the standard_prices table is also maintained in USD.
// This file is the single chokepoint where that conversion happens — every
// code path that produces a TableModelPricing row that the cost engine will
// later read runs through ApplyFX before the row reaches the database.
//
// Today's datasheet already ships USD-denominated rates (every Provider on
// the supported list bills in USD), so ApplyFX is effectively a no-op for
// non-zero rates. The shape is preserved so a future non-USD provider only
// has to set the rate and an explicit fxRate value here — the rest of the
// engine reads already-USD rows and never needs to know about the source
// currency.

// PricingCurrency is the ISO-4217-style currency code that every
// TableModelPricing row is stored in. D11-A pins this to USD.
const PricingCurrency = "USD"

// ApplyFXUSD converts a per-token rate to the currency the cost engine reads.
// Under D11-A the engine always reads USD, so a USD rate is returned
// unchanged and a non-USD rate is multiplied by the explicit fxRate. We
// keep the call site (every TableModelPricing write) honest by routing
// every rate through this helper — no caller may persist a non-USD rate
// without going through the conversion.
//
// fxRate == 0 is treated as "no fx configured" and falls through as USD.
// Callers should populate fxRate explicitly when they know the source
// currency so future currency refactors can audit the conversion.
func ApplyFXUSD(rate, fxRate float64) float64 {
	if rate == 0 {
		return 0
	}
	if fxRate <= 0 {
		// No fx configured → assume already-USD (the current datasheet
		// reality). Returning the rate unchanged keeps ApplyFX a no-op for
		// the existing pipeline while leaving the hook in place.
		return rate
	}
	return rate * fxRate
}
