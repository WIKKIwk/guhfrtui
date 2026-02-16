package regions

// Region represents one UHF regulatory preset.
type Region struct {
	Code string
	Name string
	Band string
}

var Catalog = []Region{
	{Code: "US", Name: "United States", Band: "902-928 MHz"},
	{Code: "EU", Name: "Europe", Band: "865-868 MHz"},
	{Code: "CN840", Name: "China 840", Band: "840-845 MHz"},
	{Code: "CN920", Name: "China 920", Band: "920-925 MHz"},
	{Code: "JP", Name: "Japan", Band: "916.8-923.4 MHz"},
	{Code: "KR", Name: "Korea", Band: "917-923.5 MHz"},
	{Code: "IN", Name: "India", Band: "865-867 MHz"},
	{Code: "AU", Name: "Australia", Band: "920-926 MHz"},
	{Code: "NZ", Name: "New Zealand", Band: "922-928 MHz"},
	{Code: "RU", Name: "Russia", Band: "866-868 MHz"},
	{Code: "BR", Name: "Brazil", Band: "902-907.5 MHz"},
	{Code: "ZA", Name: "South Africa", Band: "915-919 MHz"},
	{Code: "SG", Name: "Singapore", Band: "920-925 MHz"},
	{Code: "MY", Name: "Malaysia", Band: "919-923 MHz"},
	{Code: "TH", Name: "Thailand", Band: "920-925 MHz"},
	{Code: "VN", Name: "Vietnam", Band: "920-923 MHz"},
}

func DefaultIndex() int {
	for i, region := range Catalog {
		if region.Code == "US" {
			return i
		}
	}
	return 0
}
