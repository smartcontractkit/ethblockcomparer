# ethblockcomparer

## How to use

```bash
go run main.go <endpoint1> <endpoint2> <difference_threshold>

# e.g.
go run main.go localhost:18545 https://ropsten.geth.chain.link:8545 2

# if you want to skip CA authenticity check for self-signed certs:
INSECURE_SKIP_VERIFY=true go run main.go localhost:18545 https://ropsten.geth.chain.link:8545 2
```

## Development

```bash
dep ensure
go test ./...
```
