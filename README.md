# go-stratum-client
Stratum client implemented in Go (WIP) forked from https://github.com/gurupras/go-stratum-client 
A huge thanks to gurupras for writing and releasing the original project

This is an extremely primitive stratum client implemented in Golang. 
Most of the code was implemented by going through the sources of cpuminer-multi and xmrig.

## What can it do?
  - Connect to pool
  - Authenticate
  - Receive jobs
  - Submit results
  - Keepalive

## What's pending
  - Full coverage of the stratum API

## andtheysay's changes
  - Use go modules
  - Replace logrus with [zap](https://github.com/uber-go/zap) because I like zap more
  - Replace json with [json-iterator](https://github.com/json-iterator/go) to encode json quicker
  - Use andtheysay's fork of [set](github.com/fatih/set). There are no functional changes, I just wanted to backup the repo because it is in archived mode
  - Updated the README
  
## Can I help?
Yes! Please send me PRs that can fix bugs, clean up the implementation and improve it.
