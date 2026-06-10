package catalog

import "testing"

func TestFilterCountriesExactCode(t *testing.T) {
	countries := Countries()

	filtered := FilterCountries(countries, "NG")
	if len(filtered) != 1 {
		t.Fatalf("expected exactly one NG result, got %d: %v", len(filtered), filtered)
	}
	code, name := SplitCountry(filtered[0])
	if code != "NG" || name != "尼日利亚" {
		t.Fatalf("unexpected NG result: code=%q name=%q label=%q", code, name, filtered[0])
	}
}

func TestFilterCountriesChineseAndEnglishNames(t *testing.T) {
	countries := Countries()

	for _, query := range []string{"美国", "United States"} {
		filtered := FilterCountries(countries, query)
		if len(filtered) == 0 {
			t.Fatalf("expected results for query %q", query)
		}
		foundUS := false
		for _, item := range filtered {
			code, _ := SplitCountry(item)
			if code == "US" {
				foundUS = true
				break
			}
		}
		if !foundUS {
			t.Fatalf("expected query %q to include US, got %v", query, filtered)
		}
	}
}

func TestCityOptions(t *testing.T) {
	options := CityOptions("NG")
	if len(options) == 0 || options[0] != CityAllOption {
		t.Fatalf("expected city options to start with all option, got %v", options)
	}
	if StringIndex(options, "Lagos") < 0 {
		t.Fatalf("expected NG city options to include Lagos, got %v", options)
	}
}

func TestDefaultCountryIndex(t *testing.T) {
	countries := Countries()
	idx := DefaultCountryIndex(countries, "JP")
	if idx < 0 || idx >= len(countries) {
		t.Fatalf("JP default index out of range: %d", idx)
	}
	code, _ := SplitCountry(countries[idx])
	if code != "JP" {
		t.Fatalf("expected JP at default index, got %q", countries[idx])
	}
}
