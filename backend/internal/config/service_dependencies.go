package config

var ServiceDependencies = map[string][]string{
	"checkout-api": {"payments-api", "inventory-api"},
	"payments-api": {"database", "fraud-service"},
	"inventory-api": {"database"},
	"fraud-service": {"ml-service"},
	"ml-service":    {},
	"database":      {},
}
