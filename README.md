# go-stratum-client
Stratum client implemented in Go (WIP) forked from https://github.com/gurupras/go-stratum-client 
<br />A huge thanks to gurupras for writing and releasing the original project

This is an extremely primitive stratum client implemented in Golang. 
Most of the code was implemented by going through the sources of cpuminer-multi and xmrig.

## What can it do?
  - Connect to pool
  - Authenticate
  - Receive jobs
  - Submit results
  - Keepalive

## How to use
To connect and authorize
```go
import (
  "os"

  stratum "github.com/andtheysay/go-stratum-client"
)

sc := stratum.New()

if err := sc.Connect(poolURL); err != nil {
  handleError(err)
  os.Exit(1)
}

if err := sc.Authorize(poolUsername, poolPassword); err != nil {
  handleError(err)
  os.Exit(1)
}
```
For a more complete example take a look at the [go-cryptonight-miner repo](https://github.com/gurupras/go-cryptonight-miner), in particular [xmrig_cpuminer.go](https://github.com/gurupras/go-cryptonight-miner/blob/ba570bd319b8e5d337570ff234255dea60591743/cpu-miner/xmrig_cpuminer.go)
## Testing
To run the tests create a test-config.yaml with the pool information
```yaml
pool:
username:
pass:
```
Then run the command
```sh
go test -run ''
```
To run a mock stratum server you can use the mock_server binary from the [releases in this repo](https://github.com/andtheysay/go-stratum-client/releases)

First download the binary for your platform and give it the required permissions
```sh
wget https://github.com/andtheysay/go-stratum-client/releases/download/1.0/mock_server-linux-amd64
chmod +x mock_server-linux-amd64
```
Then run the mock server. You can specify which port to run on with the --port or -p flag
```sh
./mock_server-linux-amd64 --port=8888
```
## What's pending
  - Full coverage of the stratum API
  - Provide an easier to understand example to showcase how to use the library

## andtheysay's changes
  - Use go modules
  - Replace logrus with [zap](https://github.com/uber-go/zap) because I like zap more
  - Replace json with [json-iterator](https://github.com/json-iterator/go) to encode json quicker
  - Use andtheysay's fork of [set](https://github.com/fatih/set). There are no functional changes, I just wanted to backup the repo because it is in archived mode
  - Updated the README to reflect my changes
  - Updated the README to explain how to use the library
  
## Can I help?
Yes! Please send me PRs that can fix bugs, clean up the implementation and improve it.
