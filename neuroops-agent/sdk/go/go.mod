module github.com/neuroops/agent/sdk/go

go 1.22

require (
	github.com/gosnmp/gosnmp v1.37.0
	golang.org/x/crypto v0.23.0
	github.com/neuroops/agent v1.0.0
)

replace github.com/neuroops/agent => ../..
